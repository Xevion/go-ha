package ha

import (
	"context"
	"time"
)

// Clock is the time source conditions read.
//
// It is declared here rather than reusing the internal one so that callers
// outside this module can build an EvalContext: a condition they cannot
// construct is a condition they cannot test, and testable automations are the
// point of taking the clock as a parameter at all.
type Clock interface {
	Now() time.Time
}

// EvalContext is what a condition is allowed to read. Everything time or state
// dependent arrives through here rather than being reached for directly, which
// is what makes conditions testable without a Home Assistant.
type EvalContext struct {
	Clock Clock
	State StateReader

	// Event is the event that fired the automation. It is the zero value when a
	// schedule fired it, so conditions that read it must tolerate that.
	Event Event
}

// Condition reports whether an automation should proceed.
//
// An error means undecided, not false. The automation's OnConditionError
// setting decides what an undecided condition does, because only the automation
// knows whether running anyway is safer than not running.
type Condition interface {
	Eval(ctx context.Context, ec EvalContext) (bool, error)
}

// ConditionFunc adapts a plain function to Condition.
type ConditionFunc func(ctx context.Context, ec EvalContext) (bool, error)

func (f ConditionFunc) Eval(ctx context.Context, ec EvalContext) (bool, error) {
	return f(ctx, ec)
}

type allCondition struct{ conditions []Condition }

// All holds when every condition holds.
func All(conditions ...Condition) Condition {
	return allCondition{conditions: conditions}
}

// Eval keeps going after an error rather than short-circuiting on it. A later
// condition that is definitely false settles the whole conjunction, and a
// definite answer is worth more than the first error.
func (c allCondition) Eval(ctx context.Context, ec EvalContext) (bool, error) {
	var undecided error

	for _, cond := range c.conditions {
		ok, err := cond.Eval(ctx, ec)
		if err != nil {
			if undecided == nil {
				undecided = err
			}
			continue
		}
		if !ok {
			return false, nil
		}
	}

	if undecided != nil {
		return false, undecided
	}
	return true, nil
}

type anyCondition struct{ conditions []Condition }

// Any holds when at least one condition holds.
func Any(conditions ...Condition) Condition {
	return anyCondition{conditions: conditions}
}

// Eval mirrors All: one condition that definitely holds settles the disjunction
// regardless of whether an earlier one could be evaluated.
func (c anyCondition) Eval(ctx context.Context, ec EvalContext) (bool, error) {
	var undecided error

	for _, cond := range c.conditions {
		ok, err := cond.Eval(ctx, ec)
		if err != nil {
			if undecided == nil {
				undecided = err
			}
			continue
		}
		if ok {
			return true, nil
		}
	}

	if undecided != nil {
		return false, undecided
	}
	return false, nil
}

type notCondition struct{ condition Condition }

// Not inverts a condition. An undecided condition stays undecided: the negation
// of "cannot tell" is still "cannot tell".
func Not(condition Condition) Condition {
	return notCondition{condition: condition}
}

func (c notCondition) Eval(ctx context.Context, ec EvalContext) (bool, error) {
	ok, err := c.condition.Eval(ctx, ec)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
