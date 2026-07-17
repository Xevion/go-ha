package services

type InputNumber struct {
	conn Sender
}

func (ib InputNumber) Set(entityId string, value float32) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "set_value"
	req.ServiceData = map[string]any{"value": value}

	return ib.conn.Send(&req)
}

func (ib InputNumber) Increment(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "increment"

	return ib.conn.Send(&req)
}

func (ib InputNumber) Decrement(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "decrement"

	return ib.conn.Send(&req)
}

func (ib InputNumber) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_number"
	req.Service = "reload"
	return ib.conn.Send(&req)
}
