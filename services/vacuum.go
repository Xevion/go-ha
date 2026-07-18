package services

type Vacuum struct {
	conn Sender
}

// Tell the vacuum cleaner to do a spot clean-up. Takes an entityId.
func (v Vacuum) CleanSpot(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "clean_spot"

	return v.conn.Send(&req)
}

// Locate the vacuum cleaner robot. Takes an entityId.
func (v Vacuum) Locate(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "locate"

	return v.conn.Send(&req)
}

// Pause the cleaning task. Takes an entityId.
func (v Vacuum) Pause(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "pause"

	return v.conn.Send(&req)
}

// Tell the vacuum cleaner to return to its dock. Takes an entityId.
func (v Vacuum) ReturnToBase(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "return_to_base"

	return v.conn.Send(&req)
}

// Send a raw command to the vacuum cleaner. Takes an entityId and an optional map that is translated into service_data.
func (v Vacuum) SendCommand(entityId VacuumID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "send_command"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return v.conn.Send(&req)
}

// Set the fan speed of the vacuum cleaner. Takes an entityId and an optional map that is translated into service_data.
func (v Vacuum) SetFanSpeed(entityId VacuumID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "set_fan_speed"

	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return v.conn.Send(&req)
}

// Start or resume the cleaning task. Takes an entityId.
func (v Vacuum) Start(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "start"

	return v.conn.Send(&req)
}

// Start, pause, or resume the cleaning task. Takes an entityId.
func (v Vacuum) StartPause(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "start_pause"

	return v.conn.Send(&req)
}

// Stop the current cleaning task. Takes an entityId.
func (v Vacuum) Stop(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "stop"

	return v.conn.Send(&req)
}

// Stop the current cleaning task and return to home. Takes an entityId.
func (v Vacuum) TurnOff(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "turn_off"

	return v.conn.Send(&req)
}

// Start a new cleaning task. Takes an entityId.
func (v Vacuum) TurnOn(entityId VacuumID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "vacuum"
	req.Service = "turn_on"

	return v.conn.Send(&req)
}
