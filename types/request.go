package types

type NotifyRequest struct {
	// Which notify service to call, such as mobile_app_sams_iphone
	ServiceName string
	Message     string
	Title       string
	Data        map[string]any
}

// SetTemperatureRequest describes a climate.set_temperature call.
//
// The temperatures are pointers so that an unset field is distinguishable from
// a deliberate zero. Zero degrees is an ordinary setpoint in Celsius, and
// treating it as absent silently dropped it from the call.
type SetTemperatureRequest struct {
	Temperature    *float32
	TargetTempHigh *float32
	TargetTempLow  *float32
	HvacMode       string
}

func (r *SetTemperatureRequest) ToJSON() map[string]any {
	m := map[string]any{}
	if r.Temperature != nil {
		m["temperature"] = *r.Temperature
	}
	if r.TargetTempHigh != nil {
		m["target_temp_high"] = *r.TargetTempHigh
	}
	if r.TargetTempLow != nil {
		m["target_temp_low"] = *r.TargetTempLow
	}
	if r.HvacMode != "" {
		m["hvac_mode"] = r.HvacMode
	}
	return m
}

// Ptr returns a pointer to v, for the optional fields above.
func Ptr[T any](v T) *T { return &v }

// Request is a message the client stamps with a connection-scoped id before
// writing it.
//
// It lives here rather than beside the client so that a package building
// requests does not have to import the transport to describe one.
type Request interface {
	SetID(id int64)
}
