package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type InputButton struct {
	conn *connect.HAConnection
}

// Press presses an input button entity.
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
