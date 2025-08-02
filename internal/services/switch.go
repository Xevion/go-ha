package services

import (
	ws "github.com/Xevion/go-ha/internal/connect"
)

/* Structs */

type Switch struct {
	conn *ws.HAConnection
}

/* Public API */

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
