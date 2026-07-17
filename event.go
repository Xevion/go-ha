package ha

import "encoding/json"

// eventEnvelope reads only the event type. Decoding it separately matters:
// integrations put arbitrary shapes in data, and a call_service event whose
// data.entity_id is a list would fail to decode against the state_changed
// schema and take the whole event with it.
type eventEnvelope struct {
	Event struct {
		EventType string `json:"event_type"`
	} `json:"event"`
}

// stateChangedPayload models a state_changed event. The states are pointers
// because Home Assistant sends a null one either side: a null old state is an
// entity appearing, a null new state is one being removed.
type stateChangedPayload struct {
	Event struct {
		Data struct {
			EntityID string    `json:"entity_id"`
			NewState *msgState `json:"new_state"`
			OldState *msgState `json:"old_state"`
		} `json:"data"`
	} `json:"event"`
}

// parseEvent decodes a delivered event. Everything but state_changed is left
// in Raw, since this package does not model the payloads of arbitrary
// integrations.
func parseEvent(raw []byte) Event {
	var envelope eventEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Event{Raw: raw}
	}

	ev := Event{Type: envelope.Event.EventType, Raw: raw}
	if ev.Type != eventStateChanged {
		return ev
	}

	var payload stateChangedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		// The type is still known and worth dispatching on, even though this
		// particular payload could not be read.
		return ev
	}

	data := payload.Event.Data
	ev.EntityID = data.EntityID
	ev.Created = data.OldState == nil
	ev.Deleted = data.NewState == nil

	if data.OldState != nil {
		ev.From = data.OldState.entityState(data.EntityID)
	}
	if data.NewState != nil {
		ev.To = data.NewState.entityState(data.EntityID)
	}
	return ev
}

func (s msgState) entityState(entityID string) EntityState {
	return EntityState{
		EntityID:    entityID,
		State:       s.State,
		Attributes:  s.Attributes,
		LastChanged: s.LastChanged,
		LastUpdated: s.LastUpdated,
	}
}

// Event is an occurrence delivered to an automation.
type Event struct {
	// Type is the Home Assistant event type, such as "state_changed".
	Type string

	// EntityID is the entity a state_changed event concerns. It is empty for
	// event types that do not concern one.
	EntityID string

	// From and To are the states either side of a state_changed event. From is
	// the zero value when Created, To when Deleted.
	From EntityState
	To   EntityState

	// Created reports an entity that did not exist before this event.
	Created bool

	// Deleted reports an entity removed from Home Assistant.
	Deleted bool

	// Raw is the undecoded payload, for event types this package does not
	// model.
	Raw []byte
}
