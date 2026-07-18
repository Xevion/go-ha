package services

type Scene struct {
	conn Sender
}

// Apply a scene. Takes map that is translated into service_data.
func (s Scene) Apply(serviceData ...map[string]any) error {
	req := NewBaseServiceRequest("")
	req.Domain = "scene"
	req.Service = "apply"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return s.conn.Send(&req)
}

// Create a scene entity. Takes an entityId and an optional map that is translated into service_data.
func (s Scene) Create(entityId SceneID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "scene"
	req.Service = "create"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return s.conn.Send(&req)
}

// Reload the scenes.
func (s Scene) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "scene"
	req.Service = "reload"

	return s.conn.Send(&req)
}

// TurnOn a scene entity. Takes an entityId and an optional map that is translated into service_data.
func (s Scene) TurnOn(entityId SceneID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "scene"
	req.Service = "turn_on"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return s.conn.Send(&req)
}
