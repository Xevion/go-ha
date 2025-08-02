package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type Switch struct {
	conn *connect.HAConnection
}

func (s Switch) TurnOn(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "turn_on"

	return s.conn.WriteMessage(req)
}

func (s Switch) Toggle(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "toggle"

	return s.conn.WriteMessage(req)
}

func (s Switch) TurnOff(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "switch"
	req.Service = "turn_off"

	return s.conn.WriteMessage(req)
}
