package services

import (
	ws "github.com/Xevion/go-ha/internal/connect"
)

/* Structs */

type InputButton struct {
	conn *ws.HAConnection
}

/* Public API */

func (ib InputButton) Press(entityId string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_button"
	req.Service = "press"

	return ib.conn.WriteMessage(req)
}

func (ib InputButton) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_button"
	req.Service = "reload"
	return ib.conn.WriteMessage(req)
}
