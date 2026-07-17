package services

import (
	"fmt"
	"time"
)

type InputDatetime struct {
	conn Sender
}

func (ib InputDatetime) Set(entityId string, value time.Time) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "input_datetime"
	req.Service = "set_datetime"
	req.ServiceData = map[string]any{
		"timestamp": fmt.Sprint(value.Unix()),
	}

	return ib.conn.Send(&req)
}

func (ib InputDatetime) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "input_datetime"
	req.Service = "reload"
	return ib.conn.Send(&req)
}
