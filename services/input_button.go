package services

type InputButton struct {
	conn Sender
}

// Press presses an input button entity.
func (ib InputButton) Press(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_button"
	req.Service = "press"

	return ib.conn.Send(&req)
}

func (ib InputButton) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_button"
	req.Service = "reload"
	return ib.conn.Send(&req)
}
