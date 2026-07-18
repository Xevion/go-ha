package services

type Cover struct {
	conn Sender
}

// Close all or specified cover. Takes an entityId.
func (c Cover) Close(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "close_cover"

	return c.conn.Send(&req)
}

// Close all or specified cover tilt. Takes an entityId.
func (c Cover) CloseTilt(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "close_cover_tilt"

	return c.conn.Send(&req)
}

// Open all or specified cover. Takes an entityId.
func (c Cover) Open(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "open_cover"

	return c.conn.Send(&req)
}

// Open all or specified cover tilt. Takes an entityId.
func (c Cover) OpenTilt(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "open_cover_tilt"

	return c.conn.Send(&req)
}

// Move to specific position all or specified cover. Takes an entityId and an optional map that is translated into service_data.
func (c Cover) SetPosition(entityId CoverID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "set_cover_position"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return c.conn.Send(&req)
}

// Move to specific position all or specified cover tilt. Takes an entityId and an optional map that is translated into service_data.
func (c Cover) SetTiltPosition(entityId CoverID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "set_cover_tilt_position"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return c.conn.Send(&req)
}

// Stop a cover entity. Takes an entityId.
func (c Cover) Stop(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "stop_cover"

	return c.conn.Send(&req)
}

// Stop a cover entity tilt. Takes an entityId.
func (c Cover) StopTilt(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "stop_cover_tilt"

	return c.conn.Send(&req)
}

// Toggle a cover open/closed. Takes an entityId.
func (c Cover) Toggle(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "toggle"

	return c.conn.Send(&req)
}

// Toggle a cover tilt open/closed. Takes an entityId.
func (c Cover) ToggleTilt(entityId CoverID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "cover"
	req.Service = "toggle_cover_tilt"

	return c.conn.Send(&req)
}
