package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type InputText struct {
	conn *connect.HAConnection
}

// Set sets the value of an input text entity.
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
