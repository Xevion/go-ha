package services

type InputBoolean struct {
	conn Sender
}

// TurnOn turns on an input boolean entity.
func (ib InputBoolean) TurnOn(entityId InputBooleanID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "input_boolean"
	req.Service = "turn_on"

	return ib.conn.Send(&req)
}

// Toggle toggles an input boolean entity.
func (ib InputBoolean) Toggle(entityId InputBooleanID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "input_boolean"
	req.Service = "toggle"

	return ib.conn.Send(&req)
}

// TurnOff turns off an input boolean entity.
func (ib InputBoolean) TurnOff(entityId InputBooleanID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "input_boolean"
	req.Service = "turn_off"
	return ib.conn.Send(&req)
}

func (ib InputBoolean) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_boolean"
	req.Service = "reload"
	return ib.conn.Send(&req)
}
