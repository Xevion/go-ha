package services

type Lock struct {
	conn Sender
}

// Lock a lock entity. Takes an entityId and an optional map that is translated into service_data.
func (l Lock) Lock(entityId LockID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "lock"
	req.Service = "lock"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return l.conn.Send(&req)
}

// Unlock a lock entity. Takes an entityId and an optional map that is translated into service_data.
func (l Lock) Unlock(entityId LockID, serviceData ...map[string]any) error {
	req := NewBaseServiceRequest(string(entityId))
	req.Domain = "lock"
	req.Service = "unlock"
	if len(serviceData) != 0 {
		req.ServiceData = serviceData[0]
	}

	return l.conn.Send(&req)
}
