package services

type Switch struct {
	conn Sender
}

// TurnOn turns on a switch entity.
func (s Switch) TurnOn(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "turn_on"

	return s.conn.Send(&req)
}

// Toggle toggles a switch entity.
func (s Switch) Toggle(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "toggle"

	return s.conn.Send(&req)
}

// TurnOff turns off a switch entity.
func (s Switch) TurnOff(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "turn_off"

	return s.conn.Send(&req)
}
