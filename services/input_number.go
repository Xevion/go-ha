package services

type InputNumber struct {
	conn Sender
}

func (ib InputNumber) Set(entityId InputNumberID, value float32) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "input_number"
	req.Service = "set_value"
	req.ServiceData = map[string]any{"value": value}

	return ib.conn.Send(&req)
}

func (ib InputNumber) Increment(entityId InputNumberID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "input_number"
	req.Service = "increment"

	return ib.conn.Send(&req)
}

func (ib InputNumber) Decrement(entityId InputNumberID) error {
	req := NewBaseServiceRequest(string(entityId))
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
