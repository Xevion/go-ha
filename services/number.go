package services

type Number struct {
	conn Sender
}

func (ib Number) SetValue(entityId string, value float32) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "number"
	req.Service = "set_value"
	req.ServiceData = map[string]any{"value": value}

	return ib.conn.Send(&req)
}
