package ha

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
