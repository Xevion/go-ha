package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type InputNumber struct {
	conn *connect.HAConnection
}

func (ib InputNumber) Set(entityId string, value float32) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "set_value"
	req.ServiceData = map[string]any{"value": value}

	return ib.conn.WriteMessage(req)
}

func (ib InputNumber) Increment(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "increment"

	return ib.conn.WriteMessage(req)
}

func (ib InputNumber) Decrement(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_number"
	req.Service = "decrement"

	return ib.conn.WriteMessage(req)
}

func (ib InputNumber) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_number"
	req.Service = "reload"
	return ib.conn.WriteMessage(req)
}
