package ha

import "encoding/json"

// parseEvent decodes a delivered event. Everything but state_changed is left
// in Raw, since this package does not model the payloads of arbitrary
// integrations.
func parseEvent(raw []byte) Event {
	var msg stateChangedMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Event{Raw: raw}
	}

	ev := Event{Type: msg.Event.EventType, Raw: raw}
	if ev.Type != eventStateChanged {
		return ev
	}

	data := msg.Event.Data
	ev.EntityID = data.EntityID
	ev.From = EntityState{
		EntityID:    data.EntityID,
		State:       data.OldState.State,
		Attributes:  data.OldState.Attributes,
		LastChanged: data.OldState.LastChanged,
	}
	ev.To = EntityState{
		EntityID:    data.EntityID,
		State:       data.NewState.State,
		Attributes:  data.NewState.Attributes,
		LastChanged: data.NewState.LastChanged,
	}
	return ev
}

// Event is an occurrence delivered to an automation.
type Event struct {
	// Type is the Home Assistant event type, such as "state_changed".
	Type string

	// EntityID is the entity a state_changed event concerns. It is empty for
	// event types that do not concern one.
	EntityID string

	// From and To are the states either side of a state_changed event.
	From EntityState
	To   EntityState

	// Raw is the undecoded payload, for event types this package does not
	// model.
	Raw []byte
}
