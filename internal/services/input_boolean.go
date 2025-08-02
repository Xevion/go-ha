package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type InputBoolean struct {
	conn *connect.HAConnection
}

func (ib InputBoolean) TurnOn(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_boolean"
	req.Service = "turn_on"

	return ib.conn.WriteMessage(req)
}

func (ib InputBoolean) Toggle(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_boolean"
	req.Service = "toggle"

	return ib.conn.WriteMessage(req)
}

func (ib InputBoolean) TurnOff(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_boolean"
	req.Service = "turn_off"
	return ib.conn.WriteMessage(req)
}

func (ib InputBoolean) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_boolean"
	req.Service = "reload"
	return ib.conn.WriteMessage(req)
}
