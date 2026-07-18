package services

type Timer struct {
	conn Sender
}

// See https://www.home-assistant.io/integrations/timer/#action-timerstart
func (t Timer) Start(entityId TimerID, duration string) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "timer"
	req.Service = "start"
	req.ServiceData = map[string]any{
		"duration": duration,
	}

	return t.conn.Send(&req)
}

// See https://www.home-assistant.io/integrations/timer/#action-timerstart
func (t Timer) Change(entityId TimerID, duration string) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "timer"
	req.Service = "change"
	req.ServiceData = map[string]any{
		"duration": duration,
	}

	return t.conn.Send(&req)
}

// See https://www.home-assistant.io/integrations/timer/#action-timerpause
func (t Timer) Pause(entityId TimerID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "timer"
	req.Service = "pause"
	return t.conn.Send(&req)
}

// See https://www.home-assistant.io/integrations/timer/#action-timercancel
func (t Timer) Cancel(entityId TimerID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "timer"
	req.Service = "cancel"
	return t.conn.Send(&req)
}

// See https://www.home-assistant.io/integrations/timer/#action-timerfinish
func (t Timer) Finish(entityId TimerID) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "timer"
	req.Service = "finish"
	return t.conn.Send(&req)
}

// See https://www.home-assistant.io/integrations/timer/#action-timerreload
func (t Timer) Reload() error {
	req := NewBaseServiceRequest("")
	req.Domain = "timer"
	req.Service = "reload"
	return t.conn.Send(&req)
}
