package services

import (
	"github.com/Xevion/go-ha/internal/connect"
)

type ZWaveJS struct {
	conn *connect.HAConnection
}

// ZWaveJS bulk_set_partial_config_parameters service.
func (zw ZWaveJS) BulkSetPartialConfigParam(entityId string, parameter int, value any) error {
	req := NewBaseServiceRequest(entityId)
	req.Domain = "zwave_js"
	req.Service = "bulk_set_partial_config_parameters"
	req.ServiceData = map[string]any{
		"parameter": parameter,
		"value":     value,
	}

	return zw.conn.WriteMessage(req)
}
