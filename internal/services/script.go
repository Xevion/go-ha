package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type Script struct {
	conn *connect.HAConnection
}

// Reload a script that was created in the HA UI.
func (s Script) Reload(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "script"
	req.Service = "reload"

	return s.conn.WriteMessage(req)
}

// Toggle a script that was created in the HA UI.
func (s Script) Toggle(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "script"
	req.Service = "toggle"

	return s.conn.WriteMessage(req)
}

// TurnOff a script that was created in the HA UI.
func (s Script) TurnOff() error {
	req := NewBaseServiceRequest("")
	req.Domain = "script"
	req.Service = "turn_off"

	return s.conn.WriteMessage(req)
}

// TurnOn a script that was created in the HA UI.
func (s Script) TurnOn(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "script"
	req.Service = "turn_on"

	return s.conn.WriteMessage(req)
}
