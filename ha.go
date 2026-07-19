package ha

import (
	"context"
	"time"

	"github.com/Xevion/go-ha/core"
	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
	"github.com/Xevion/go-ha/types"
)

// Version is the go-ha release this build corresponds to. It is also sent as
// the User-Agent on every REST call.
const Version = internal.Version

// SunEntityID is the entity Home Assistant publishes solar times on.
const SunEntityID = core.SunEntityID

// Errors this package returns, so a caller can classify a failure with
// errors.Is rather than matching on message text.
var (
	// ErrInvalidArgs reports a malformed NewAppRequest.
	ErrInvalidArgs = core.ErrInvalidArgs

	// ErrConnectionAbandoned reports that the client gave up re-establishing
	// the connection, so Start returned without being asked to.
	ErrConnectionAbandoned = core.ErrConnectionAbandoned

	// ErrNotRunning reports Start called twice, or after Close.
	ErrNotRunning = core.ErrNotRunning

	// ErrInvalidAutomation reports an automation that cannot be built.
	ErrInvalidAutomation = core.ErrInvalidAutomation

	// ErrInvalidTimeOfDay reports an hour or minute outside its range.
	ErrInvalidTimeOfDay = core.ErrInvalidTimeOfDay

	// ErrEntityNotFound reports an entity Home Assistant does not know about.
	ErrEntityNotFound = internal.ErrEntityNotFound

	// ErrUnauthorized reports a token Home Assistant refused.
	ErrUnauthorized = internal.ErrUnauthorized

	// ErrHTTPStatus reports any other unsuccessful REST response.
	ErrHTTPStatus = internal.ErrHttpStatus

	// ErrNotConnected reports a call made while the websocket was down.
	ErrNotConnected = connect.ErrNotConnected

	// ErrAuthFailed reports a rejected websocket handshake.
	ErrAuthFailed = connect.ErrAuthFailed
)

// Condition reports whether an automation should run.
//
// An error means undecided rather than false: All and Any do not short-circuit
// on one, so a definite answer from any branch still settles the expression.
// What happens when nothing settles it is the automation's choice, set with
// [AutomationBuilder.OnConditionError].
type Condition interface {
	Eval(ctx context.Context, ec EvalContext) (bool, error)
}

// StateReader reads entity state from the local cache, falling back to Home
// Assistant until the first snapshot has landed.
type StateReader interface {
	ListEntities() ([]EntityState, error)
	Get(entityId string) (EntityState, error)
	Equals(entityId, state string) (bool, error)
}

// EntityRef is anything that names an entity: a plain string, or one of the
// domain-typed ids cmd/generate emits.
//
// The trigger and condition constructors are generic over it so generated
// constants can be used directly. Service methods cannot be, since Go does not
// allow type parameters on methods, which is why they take their domain's id
// type exactly.
type EntityRef interface{ ~string }

// The automation grammar: a trigger fires, conditions decide, policy governs
// concurrency, and the action runs.
type (
	// Automation is a trigger, a condition, a policy and an action, which
	// together describe one rule. Build one with [NewAutomation].
	Automation = core.Automation

	// AutomationBuilder accumulates an automation. Every stage returns a copy,
	// so a shared prefix can be held in a variable and branched.
	AutomationBuilder = core.AutomationBuilder

	// Action is the work an automation does when it fires.
	Action = core.Action

	// Run is the context an action is given when it fires.
	Run = core.Run

	// EvalContext is what a condition is evaluated against.
	EvalContext = core.EvalContext

	// ConditionFunc adapts a plain function to [Condition].
	ConditionFunc = core.ConditionFunc

	// ConditionErrorPolicy decides what an automation does when a condition
	// cannot be evaluated.
	ConditionErrorPolicy = core.ConditionErrorPolicy

	// Policy governs how runs of one automation relate to each other.
	Policy = core.Policy

	// Mode mirrors Home Assistant's automation mode.
	Mode = core.Mode
)

// Triggers. The two families are united by [Trigger] so one automation can hold
// a mixed list, which is what lets it express Home Assistant's real grammar.
type (
	// Trigger is either a [ScheduleTrigger] or an [EventTrigger]. It is closed:
	// the families are implemented in this module.
	Trigger = core.Trigger

	// ScheduleTrigger fires at a time it computes.
	ScheduleTrigger = core.ScheduleTrigger

	// EventTrigger fires on events it has subscribed to.
	EventTrigger = core.EventTrigger

	// StateChangeTrigger fires when an entity changes state. Narrow it with
	// From, To and For.
	StateChangeTrigger = core.StateChangeTrigger

	// EventTypeTrigger fires on Home Assistant events by type.
	EventTypeTrigger = core.EventTypeTrigger

	// Subscription declares what an event trigger needs delivered.
	Subscription = core.Subscription

	// SunEvent names one of the solar times Home Assistant publishes.
	SunEvent = core.SunEvent

	// ClockTime is a time of day, built with [TimeOfDay].
	ClockTime = core.ClockTime
)

// The app, its state and its services.
type (
	// App owns the connection and runs the registered automations.
	App = core.App

	// Service calls back into Home Assistant.
	Service = core.Service

	// EntityState is one entity's state and attributes.
	EntityState = core.EntityState

	// Event is a Home Assistant event delivered to a trigger or an action.
	Event = core.Event

	// Clock is the time source, injectable so automations can be tested.
	Clock = types.Clock
)

// Modes, matching Home Assistant's automation mode.
const (
	// ModeSingle drops a trigger arriving while a run is in flight.
	ModeSingle = core.ModeSingle

	// ModeRestart cancels the running action and starts again.
	ModeRestart = core.ModeRestart

	// ModeQueued runs them in order, one at a time.
	ModeQueued = core.ModeQueued

	// ModeParallel runs them concurrently, up to Limit.
	ModeParallel = core.ModeParallel
)

// What to do when a condition cannot be evaluated.
const (
	// SkipRun treats an unevaluable condition as a reason not to run.
	SkipRun = core.SkipRun

	// RunAnyway runs despite one, for automations where not acting is the more
	// dangerous outcome.
	RunAnyway = core.RunAnyway
)

// The solar events read from sun.sun.
const (
	SunRising  = core.SunRising
	SunSetting = core.SunSetting
	SunDawn    = core.SunDawn
	SunDusk    = core.SunDusk
)

// NewApp connects to Home Assistant and returns an app to register automations
// on. Call [App.Start] to run it.
func NewApp(request types.NewAppRequest) (*App, error) { return core.NewApp(request) }

// NewAutomation starts building an automation. The name appears in logs.
func NewAutomation(name string) AutomationBuilder { return core.NewAutomation(name) }

// Daily fires once a day at the given time.
func Daily(at ClockTime) ScheduleTrigger { return core.Daily(at) }

// Every fires on a fixed interval.
func Every(interval time.Duration) ScheduleTrigger { return core.Every(interval) }

// Cron fires on a cron expression.
func Cron(expression string) ScheduleTrigger { return core.Cron(expression) }

// AtStartup fires once, when the app starts.
func AtStartup() ScheduleTrigger { return core.AtStartup() }

// Sunrise fires when the sun rises, optionally offset. A negative offset fires
// before the event.
func Sunrise(offset ...time.Duration) ScheduleTrigger { return core.Sunrise(offset...) }

// Sunset fires when the sun sets, optionally offset.
func Sunset(offset ...time.Duration) ScheduleTrigger { return core.Sunset(offset...) }

// Dawn fires at the start of civil twilight, optionally offset.
func Dawn(offset ...time.Duration) ScheduleTrigger { return core.Dawn(offset...) }

// Dusk fires at the end of civil twilight, optionally offset.
func Dusk(offset ...time.Duration) ScheduleTrigger { return core.Dusk(offset...) }

// StateChanged fires when any of the given entities changes state. With no
// entities it fires on every state change, which is rarely what you want.
func StateChanged[T EntityRef](entityIDs ...T) StateChangeTrigger {
	return core.StateChanged(entityIDs...)
}

// EventFired fires on any of the given Home Assistant event types, for events
// this package does not model directly.
func EventFired(eventTypes ...string) EventTypeTrigger { return core.EventFired(eventTypes...) }

// TimeOfDay is a wall-clock time. An hour or minute out of range fails the
// build rather than panicking when the automation fires.
func TimeOfDay(hour, minute int) ClockTime { return core.TimeOfDay(hour, minute) }

// All holds when every condition holds.
func All(conditions ...Condition) Condition { return core.All(toCore(conditions)...) }

// Any holds when at least one condition holds.
func Any(conditions ...Condition) Condition { return core.Any(toCore(conditions)...) }

// Not inverts a condition, leaving an unevaluable one unevaluable.
func Not(condition Condition) Condition { return core.Not(condition) }

// StateIs holds while the entity is in the given state.
func StateIs[T EntityRef](entityID T, state string) Condition { return core.StateIs(entityID, state) }

// StateIsNot holds while the entity is in any other state.
func StateIsNot[T EntityRef](entityID T, state string) Condition {
	return core.StateIsNot(entityID, state)
}

// StateIsOneOf holds while the entity is in any of the given states.
func StateIsOneOf[T EntityRef](entityID T, states ...string) Condition {
	return core.StateIsOneOf(entityID, states...)
}

// TimeBetween holds between two times of day, and may cross midnight.
func TimeBetween(start, end ClockTime) Condition { return core.TimeBetween(start, end) }

// AfterTime holds from the given time of day until midnight.
func AfterTime(t ClockTime) Condition { return core.AfterTime(t) }

// BeforeTime holds from midnight until the given time of day.
func BeforeTime(t ClockTime) Condition { return core.BeforeTime(t) }

// OnWeekdays holds on the given days of the week.
func OnWeekdays(days ...time.Weekday) Condition { return core.OnWeekdays(days...) }

// OnDates holds on the given calendar dates, ignoring their time of day.
func OnDates(dates ...time.Time) Condition { return core.OnDates(dates...) }

// InDateRange holds between two dates, inclusive.
func InDateRange(start, end time.Time) Condition { return core.InDateRange(start, end) }

// SunIsUp holds while Home Assistant reports the sun above the horizon.
func SunIsUp() Condition { return core.SunIsUp() }

// SunIsDown holds while Home Assistant reports the sun below the horizon.
func SunIsDown() Condition { return core.SunIsDown() }

// toCore converts a slice of the locally declared Condition to the one core
// takes. The interfaces are identical, so this is a copy rather than a
// conversion, and it stops compiling the moment they diverge.
func toCore(conditions []Condition) []core.Condition {
	out := make([]core.Condition, len(conditions))
	for i, c := range conditions {
		out[i] = c
	}
	return out
}
