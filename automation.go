package ha

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Xevion/go-ha/internal"
)

// ErrInvalidAutomation reports an automation that cannot be built.
var ErrInvalidAutomation = errors.New("invalid automation")

// Run is the context an action is given when it fires.
type Run struct {
	// Services calls back into Home Assistant.
	Services *Service

	// State reads entity state.
	State StateReader

	// Event is the event that fired the automation. It is the zero value when
	// a schedule fired it.
	Event Event

	// Trigger is the trigger that fired, of the several an automation may hold.
	Trigger Trigger
}

// Action is the work an automation does. Returning an error logs it; it does
// not stop the automation from firing again.
type Action func(ctx context.Context, run Run) error

// ConditionErrorPolicy decides what an automation does when a condition cannot
// be evaluated, such as when the entity it reads is unreachable.
type ConditionErrorPolicy int

const (
	// SkipRun treats an unevaluable condition as a reason not to run.
	SkipRun ConditionErrorPolicy = iota

	// RunAnyway runs despite an unevaluable condition, for automations where
	// not acting is the more dangerous outcome.
	RunAnyway
)

// Automation is a trigger, a condition, a policy and an action, which together
// describe one rule. Build one with NewAutomation.
type Automation struct {
	name             string
	triggers         []Trigger
	condition        Condition
	policy           Policy
	action           Action
	onConditionError ConditionErrorPolicy

	// runtime is allocated by Build, never by the builder stages. Every stage
	// returns a copy, so allocating earlier would hand one runner to every
	// automation branched off a shared prefix.
	runtime *runner
}

func (a Automation) Name() string { return a.name }

func (a Automation) String() string {
	return fmt.Sprintf("%s (%d trigger(s), %s)", a.name, len(a.triggers), a.policy.Mode)
}

// validator is implemented by triggers and conditions that can be constructed
// in an invalid state. Build collects what they report so a bad argument
// surfaces once, at build time, rather than as a panic at fire time.
type validator interface{ validate() error }

// AutomationBuilder accumulates an automation. Every stage returns a copy, so a
// shared prefix can be held in a variable, or returned from a function, and
// branched into several automations.
type AutomationBuilder struct {
	a    Automation
	errs []error
}

// NewAutomation starts building an automation. The name appears in logs.
func NewAutomation(name string) AutomationBuilder {
	return AutomationBuilder{a: Automation{name: name}}
}

// On adds triggers. An automation fires when any of them fires, and they may
// mix the schedule and event families freely.
func (b AutomationBuilder) On(triggers ...Trigger) AutomationBuilder {
	// Copied rather than appended in place: two automations branched off this
	// builder would otherwise write into the same backing array.
	b.a.triggers = concat(b.a.triggers, triggers)
	return b
}

// When adds conditions, all of which must hold. Compose with All, Any and Not
// for anything more involved.
func (b AutomationBuilder) When(conditions ...Condition) AutomationBuilder {
	existing := b.a.condition
	combined := All(conditions...)
	if existing != nil {
		combined = All(existing, combined)
	}
	b.a.condition = combined
	return b
}

// Mode sets what happens when a trigger arrives while a run is in flight.
func (b AutomationBuilder) Mode(m Mode) AutomationBuilder {
	b.a.policy.Mode = m
	return b
}

// Throttle drops triggers arriving within d of the last admitted one, counted
// separately for each entity.
func (b AutomationBuilder) Throttle(d time.Duration) AutomationBuilder {
	b.a.policy.Throttle = d
	return b
}

// Limit caps in-flight runs under ModeParallel and waiting runs under
// ModeQueued.
func (b AutomationBuilder) Limit(n int) AutomationBuilder {
	b.a.policy.Limit = n
	return b
}

// OnConditionError decides what happens when a condition cannot be evaluated.
func (b AutomationBuilder) OnConditionError(p ConditionErrorPolicy) AutomationBuilder {
	b.a.onConditionError = p
	return b
}

// Do sets the action.
func (b AutomationBuilder) Do(action Action) AutomationBuilder {
	b.a.action = action
	return b
}

// Build produces the automation, reporting everything wrong with it at once.
func (b AutomationBuilder) Build() (Automation, error) {
	errs := concat(nil, b.errs)

	if b.a.name == "" {
		errs = append(errs, errors.New("automation needs a name"))
	}
	if len(b.a.triggers) == 0 {
		errs = append(errs, errors.New("automation needs at least one trigger, set with On"))
	}
	if b.a.action == nil {
		errs = append(errs, errors.New("automation needs an action, set with Do"))
	}

	for _, t := range b.a.triggers {
		if v, ok := t.(validator); ok {
			if err := v.validate(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	errs = append(errs, validateCondition(b.a.condition))

	if err := errors.Join(errs...); err != nil {
		return Automation{}, fmt.Errorf("%w %q: %w", ErrInvalidAutomation, b.a.name, err)
	}

	built := b.a
	built.runtime = newRunner(built.policy, internal.RealClock{})
	return built, nil
}

// MustBuild builds the automation and panics if it cannot. It is for package
// level declarations, where there is no error to return and a misconfigured
// automation should stop the program before it starts.
func (b AutomationBuilder) MustBuild() Automation {
	a, err := b.Build()
	if err != nil {
		panic(err)
	}
	return a
}

// validateCondition walks a condition tree, since combinators hold the leaves
// that know whether their arguments made sense.
func validateCondition(c Condition) error {
	switch v := c.(type) {
	case nil:
		return nil
	case allCondition:
		return validateAll(v.conditions)
	case anyCondition:
		return validateAll(v.conditions)
	case notCondition:
		return validateCondition(v.condition)
	case validator:
		return v.validate()
	}
	return nil
}

func validateAll(conditions []Condition) error {
	var errs []error
	for _, c := range conditions {
		errs = append(errs, validateCondition(c))
	}
	return errors.Join(errs...)
}

// concat returns a new slice holding both, so builder branches never share a
// backing array.
func concat[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

// fire evaluates the conditions and, if they hold, runs the action under the
// policy. It reports whether the action was admitted.
func (a Automation) fire(ctx context.Context, ec EvalContext, deps Run, key string) bool {
	if a.condition != nil {
		ok, err := a.condition.Eval(ctx, ec)
		if err != nil {
			if a.onConditionError == SkipRun {
				slog.Warn("Skipping automation, condition could not be evaluated",
					"automation", a.name, "error", err)
				return false
			}
			slog.Warn("Running automation despite an unevaluable condition",
				"automation", a.name, "error", err)
		} else if !ok {
			return false
		}
	}

	return a.runtime.run(ctx, key, func(runCtx context.Context) {
		if err := a.action(runCtx, deps); err != nil {
			slog.Error("Automation action failed", "automation", a.name, "error", err)
		}
	})
}
