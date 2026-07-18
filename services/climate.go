package services

import (
	"github.com/Xevion/go-ha/types"
)

type Climate struct {
	conn Sender
}

func (c Climate) SetFanMode(entityId ClimateID, fanMode string) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "climate"
	req.Service = "set_fan_mode"
	req.ServiceData = map[string]any{"fan_mode": fanMode}

	return c.conn.Send(&req)
}

func (c Climate) SetTemperature(entityId ClimateID, serviceData types.SetTemperatureRequest) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "climate"
	req.Service = "set_temperature"
	req.ServiceData = serviceData.ToJSON()

	return c.conn.Send(&req)
}
