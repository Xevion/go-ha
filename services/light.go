package services

type Light struct {
	conn Sender
}

// TurnOn a light entity. Takes an entityId and an optional map that is translated into service_data.
func (l Light) TurnOn(entityId LightID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "light"
	req.Service = "turn_on"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return l.conn.Send(&req)
}

// Toggle a light entity. Takes an entityId and an optional map that is translated into service_data.
func (l Light) Toggle(entityId LightID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "light"
	req.Service = "toggle"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return l.conn.Send(&req)
}

// TurnOff turns off a light entity.
func (l Light) TurnOff(entityId LightID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "light"
	req.Service = "turn_off"
	return l.conn.Send(&req)
}
