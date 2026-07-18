package ha

import (
	"context"
	"fmt"
	"time"
)

// SunEntityID is the entity Home Assistant publishes solar times on.
const SunEntityID = "sun.sun"

// SunEvent names one of the solar times Home Assistant publishes.
type SunEvent int

const (
	SunRising SunEvent = iota
	SunSetting
	SunDawn
	SunDusk
)

func (e SunEvent) attribute() string {
	switch e {
	case SunSetting:
		return "next_setting"
	case SunDawn:
		return "next_dawn"
	case SunDusk:
		return "next_dusk"
	default:
		return "next_rising"
	}
}

func (e SunEvent) String() string {
	switch e {
	case SunSetting:
		return "sunset"
	case SunDawn:
		return "dawn"
	case SunDusk:
		return "dusk"
	default:
		return "sunrise"
	}
}

// sunTrigger fires at a solar time read from sun.sun.
//
// The times come from Home Assistant rather than being computed here. It runs
// astral against the observer's latitude, longitude AND elevation, with a
// configurable solar depression for dawn and dusk. Computing them locally
// means quietly disagreeing with the times on the user's own dashboard.
type sunTrigger struct {
	event  SunEvent
	offset time.Duration

	// state is bound at registration. A trigger is declared before an App
	// exists, so it has nothing to read until it joins one.
	state StateReader
}

// Sunrise fires when the sun rises, optionally offset. A negative offset fires
// before the event.
func Sunrise(offset ...time.Duration) ScheduleTrigger { return newSunTrigger(SunRising, offset) }

// Sunset fires when the sun sets, optionally offset.
func Sunset(offset ...time.Duration) ScheduleTrigger { return newSunTrigger(SunSetting, offset) }

// Dawn fires at the start of civil twilight, optionally offset.
func Dawn(offset ...time.Duration) ScheduleTrigger { return newSunTrigger(SunDawn, offset) }

// Dusk fires at the end of civil twilight, optionally offset.
func Dusk(offset ...time.Duration) ScheduleTrigger { return newSunTrigger(SunDusk, offset) }

func newSunTrigger(event SunEvent, offset []time.Duration) ScheduleTrigger {
	t := &sunTrigger{event: event}
	if len(offset) > 0 {
		t.offset = offset[0]
	}
	return t
}

func (t *sunTrigger) trigger() {}

// bind gives the trigger the reader it derives its times from.
func (t *sunTrigger) bind(state StateReader) { t.state = state }

// dynamic reports that this trigger's times move independently of it firing,
// so the scheduler re-derives them whenever sun.sun changes.
func (t *sunTrigger) dynamic() bool { return true }

func (t *sunTrigger) NextTime(after time.Time) (time.Time, bool) {
	if t.state == nil {
		return time.Time{}, false
	}

	sun, err := t.state.Get(SunEntityID)
	if err != nil {
		return time.Time{}, false
	}

	raw, ok := sun.Attributes[t.event.attribute()].(string)
	if !ok {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}

	next := parsed.Local().Add(t.offset)
	if next.After(after) {
		return next, true
	}

	// Asked for the occurrence after the one that just fired. Home Assistant
	// publishes the following time when the event happens, and it has not
	// reached us yet, so this is provisional: the refresh that the sun.sun
	// update triggers replaces it with the real value moments later.
	return next.AddDate(0, 0, 1), true
}

func (t *sunTrigger) String() string {
	if t.offset == 0 {
		return t.event.String()
	}
	// Duration renders its own minus sign but never a plus, and the + flag does
	// nothing for a string verb.
	if t.offset > 0 {
		return fmt.Sprintf("%s+%s", t.event, t.offset)
	}
	return fmt.Sprintf("%s%s", t.event, t.offset)
}

type sunUpCondition struct{ up bool }

// SunIsUp holds while Home Assistant reports the sun above the horizon.
func SunIsUp() Condition { return sunUpCondition{up: true} }

// SunIsDown holds while Home Assistant reports the sun below the horizon.
func SunIsDown() Condition { return sunUpCondition{up: false} }

func (c sunUpCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	sun, err := ec.State.Get(SunEntityID)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", SunEntityID, err)
	}
	return (sun.State == "above_horizon") == c.up, nil
}

func (c sunUpCondition) String() string {
	if c.up {
		return "sun is up"
	}
	return "sun is down"
}
