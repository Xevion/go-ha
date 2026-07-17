package services

type InputText struct {
	conn Sender
}

// Set sets the value of an input text entity.
func (ib InputText) Set(entityId string, value string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_text"
	req.Service = "set_value"
	req.ServiceData = map[string]any{
		"value": value,
	}

	return ib.conn.Send(&req)
}

func (ib InputText) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_text"
	req.Service = "reload"
	return ib.conn.Send(&req)
}
