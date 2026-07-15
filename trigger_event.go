package ha

import (
	"fmt"
	"slices"
	"strings"
)

const eventStateChanged = "state_changed"

// StateChangeTrigger fires when an entity changes state. Build one with
// StateChanged and narrow it with From and To.
type StateChangeTrigger struct {
	entityIDs []string
	from      string
	to        string
}

// StateChanged fires when any of the given entities changes state. With no
// entities it fires on every state change, which is rarely what you want.
func StateChanged(entityIDs ...string) StateChangeTrigger {
	return StateChangeTrigger{entityIDs: entityIDs}
}

// From narrows the trigger to transitions out of the given state.
func (t StateChangeTrigger) From(state string) StateChangeTrigger {
	t.from = state
	return t
}

// To narrows the trigger to transitions into the given state.
func (t StateChangeTrigger) To(state string) StateChangeTrigger {
	t.to = state
	return t
}

func (t StateChangeTrigger) trigger() {}

func (t StateChangeTrigger) Subscriptions() []Subscription {
	return []Subscription{{EventType: eventStateChanged}}
}

func (t StateChangeTrigger) Matches(ev Event) bool {
	if ev.Type != eventStateChanged {
		return false
	}

	// Home Assistant emits a state_changed whenever attributes move too. A
	// transition to the state it already held is not a change worth firing on.
	if ev.To.State == ev.From.State {
		return false
	}

	if len(t.entityIDs) > 0 && !slices.Contains(t.entityIDs, ev.EntityID) {
		return false
	}
	if t.from != "" && ev.From.State != t.from {
		return false
	}
	if t.to != "" && ev.To.State != t.to {
		return false
	}
	return true
}

func (t StateChangeTrigger) String() string {
	s := "state change on " + strings.Join(t.entityIDs, ", ")
	if t.from != "" {
		s += " from " + t.from
	}
	if t.to != "" {
		s += " to " + t.to
	}
	return s
}

// EventTypeTrigger fires on Home Assistant events by type.
type EventTypeTrigger struct {
	eventTypes []string
}

// EventFired fires on any of the given Home Assistant event types, for events
// this package does not model directly.
func EventFired(eventTypes ...string) EventTypeTrigger {
	return EventTypeTrigger{eventTypes: eventTypes}
}

func (t EventTypeTrigger) trigger() {}

func (t EventTypeTrigger) Subscriptions() []Subscription {
	subs := make([]Subscription, 0, len(t.eventTypes))
	for _, et := range t.eventTypes {
		subs = append(subs, Subscription{EventType: et})
	}
	return subs
}

func (t EventTypeTrigger) Matches(ev Event) bool {
	return slices.Contains(t.eventTypes, ev.Type)
}

func (t EventTypeTrigger) validate() error {
	if len(t.eventTypes) == 0 {
		return fmt.Errorf("%w: EventFired needs at least one event type", ErrInvalidArgs)
	}
	return nil
}

func (t EventTypeTrigger) String() string {
	return "event " + strings.Join(t.eventTypes, ", ")
}
