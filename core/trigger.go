package core

import (
	"fmt"
	"time"

	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/Xevion/go-ha/types"
)

// Trigger is what starts an automation.
//
// Every trigger belongs to one of two families, and the marker method keeps
// that set closed: ScheduleTrigger is asked when it will next fire and is
// driven from a heap, while EventTrigger declares what it needs delivered and
// is driven from the event stream. An automation may hold a mix of both, which
// is what lets one automation say "at sunset, or when the door opens".
type Trigger interface {
	trigger()
}

// ScheduleTrigger fires at times it can compute in advance.
type ScheduleTrigger interface {
	Trigger

	// NextTime reports the first time this trigger fires strictly after the
	// given instant. The bool is false when it will not fire again.
	NextTime(after time.Time) (time.Time, bool)
}

// EventTrigger fires on events Home Assistant sends.
type EventTrigger interface {
	Trigger

	// Subscriptions declares what this trigger needs delivered. It is
	// declarative rather than imperative so the client can replay it after a
	// reconnect, which is exactly what an imperative subscribe cannot survive.
	Subscriptions() []Subscription

	// Matches reports whether a delivered event should fire this trigger.
	Matches(ev Event) bool
}

// Subscription declares an event type an automation needs delivered.
type Subscription struct {
	EventType string
}

// scheduleTrigger adapts the internal scheduling triggers to the public
// contract, which reports absence with a bool rather than a nil pointer.
type scheduleTrigger struct {
	inner scheduling.Trigger
	err   error
	label string
}

func (t scheduleTrigger) trigger() {}

func (t scheduleTrigger) NextTime(after time.Time) (time.Time, bool) {
	if t.inner == nil {
		return time.Time{}, false
	}
	next := t.inner.NextTime(after)
	if next == nil {
		return time.Time{}, false
	}
	return *next, true
}

func (t scheduleTrigger) validate() error { return t.err }

func (t scheduleTrigger) String() string { return t.label }

// Daily fires at the same time every day.
func Daily(at ClockTime) ScheduleTrigger {
	if at.err != nil {
		return scheduleTrigger{err: at.err, label: "daily"}
	}
	return scheduleTrigger{
		inner: &scheduling.FixedTimeTrigger{Hour: at.hour, Minute: at.minute},
		label: "daily at " + at.String(),
	}
}

// Every fires on a fixed interval.
func Every(interval time.Duration) ScheduleTrigger {
	inner, err := scheduling.NewIntervalTrigger(interval)
	if err != nil {
		return scheduleTrigger{err: err, label: "every " + interval.String()}
	}
	return scheduleTrigger{inner: inner, label: "every " + interval.String()}
}

// Cron fires on a cron expression, for schedules the other triggers cannot
// describe, such as "the first Monday of the month".
func Cron(expression string) ScheduleTrigger {
	inner, err := scheduling.NewCronTrigger(expression)
	if err != nil {
		return scheduleTrigger{
			err:   fmt.Errorf("cron %q: %w", expression, err),
			label: "cron(" + expression + ")",
		}
	}
	return scheduleTrigger{inner: inner, label: "cron(" + expression + ")"}
}

// startupTrigger fires once, when the app starts.
type startupTrigger struct{ fired bool }

// AtStartup fires once when Start is called, for automations that need to act
// on the state of the world as they find it rather than waiting for it to
// change.
func AtStartup() ScheduleTrigger { return &startupTrigger{} }

func (t *startupTrigger) trigger() {}

// NextTime reports the given instant the first time it is asked and never
// again, which fires it on the scheduler's first pass and then retires it.
func (t *startupTrigger) NextTime(after time.Time) (time.Time, bool) {
	if t.fired {
		return time.Time{}, false
	}
	t.fired = true
	return after, true
}

func (t *startupTrigger) String() string { return "startup" }

// EntityRef is anything that names an entity: a plain string, or one of the
// domain-typed ids cmd/generate emits.
//
// The trigger and condition constructors are generic over it so generated
// constants can be used directly. Service methods cannot be, since Go does not
// allow type parameters on methods, which is why they take their domain's id
// type exactly.
type EntityRef interface{ ~string }

// Clock is the time source conditions and policies read. It is an alias so
// that types.NewAppRequest can name it without this package importing itself.
type Clock = types.Clock
