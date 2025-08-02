package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

/* Structs */

type InputText struct {
	conn *connect.HAConnection
}

/* Public API */

func (ib InputText) Set(entityId string, value string) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_text"
	req.Service = "set_value"
	req.ServiceData = map[string]any{
		"value": value,
	}

	return ib.conn.WriteMessage(req)
}

func (ib InputText) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_text"
	req.Service = "reload"
	return ib.conn.WriteMessage(req)
}
