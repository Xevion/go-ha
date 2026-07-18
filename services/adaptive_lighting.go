package services

type AdaptiveLighting struct {
	conn Sender
}

// Set manual control for an adaptive lighting entity.
func (al AdaptiveLighting) SetManualControl(entityId EntityID, enabled bool) error {
	req := NewBaseServiceRequest("")
	req.Domain = "adaptive_lighting"
	req.Service = "set_manual_control"
	req.ServiceData = map[string]any{
		"entity_id":      entityId,
		"manual_control": enabled,
	}

	return al.conn.Send(&req)
}
