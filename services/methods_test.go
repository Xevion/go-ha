package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/types"
)

// reqRecorder captures any request, not just a BaseServiceRequest, so it can
// see a FireEventRequest as well.
type reqRecorder struct{ last types.Request }

func (r *reqRecorder) Send(req types.Request) error { r.last = req; return nil }

// The methods below shape service_data by hand from a typed argument, so a
// wrong key or a dropped value is a call that reaches Home Assistant and does
// nothing. Each one is pinned to the exact payload it must send.
func TestServiceMethodPayloads(t *testing.T) {
	r := &recorder{}

	tests := []struct {
		name string
		call func() error
		data map[string]any
	}{
		{
			"climate set fan mode",
			func() error { return BuildService[Climate](r).SetFanMode("climate.a", "auto") },
			map[string]any{"fan_mode": "auto"},
		},
		{
			"timer start",
			func() error { return BuildService[Timer](r).Start("timer.a", "00:01:00") },
			map[string]any{"duration": "00:01:00"},
		},
		{
			"timer change",
			func() error { return BuildService[Timer](r).Change("timer.a", "00:00:30") },
			map[string]any{"duration": "00:00:30"},
		},
		{
			"input_text set",
			func() error { return BuildService[InputText](r).Set("input_text.a", "hello") },
			map[string]any{"value": "hello"},
		},
		{
			"input_number set",
			func() error { return BuildService[InputNumber](r).Set("input_number.a", 4.5) },
			map[string]any{"value": float32(4.5)},
		},
		{
			"number set value",
			func() error { return BuildService[Number](r).SetValue("number.a", 7) },
			map[string]any{"value": float32(7)},
		},
		{
			"zwavejs bulk set",
			func() error { return BuildService[ZWaveJS](r).BulkSetPartialConfigParam("sensor.a", 3, 12) },
			map[string]any{"parameter": 3, "value": any(12)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.last = nil
			require.NoError(t, tt.call())
			require.NotNil(t, r.last)
			assert.Equal(t, tt.data, r.last.ServiceData)
		})
	}
}

// InputDatetime sends the instant as a string of Unix seconds under "timestamp".
func TestInputDatetimeSetSendsUnixTimestamp(t *testing.T) {
	r := &recorder{}
	require.NoError(t, BuildService[InputDatetime](r).Set("input_datetime.a", time.Unix(1700000000, 0)))

	require.NotNil(t, r.last)
	assert.Equal(t, map[string]any{"timestamp": "1700000000"}, r.last.ServiceData)
}

// SetTemperature carries the request's own JSON shape, so an unset field is
// omitted while a deliberate zero survives.
func TestClimateSetTemperaturePayload(t *testing.T) {
	r := &recorder{}
	req := types.SetTemperatureRequest{Temperature: types.Ptr(float32(21.5)), HvacMode: "heat"}
	require.NoError(t, BuildService[Climate](r).SetTemperature("climate.a", req))

	require.NotNil(t, r.last)
	assert.Equal(t, float32(21.5), r.last.ServiceData["temperature"])
	assert.Equal(t, "heat", r.last.ServiceData["hvac_mode"])
	assert.NotContains(t, r.last.ServiceData, "target_temp_high")
}

// SetManualControl is entity-scoped but sends the entity inside service_data
// rather than as a target, which is how the adaptive_lighting integration
// expects it.
func TestAdaptiveLightingSetManualControl(t *testing.T) {
	r := &recorder{}
	require.NoError(t, BuildService[AdaptiveLighting](r).SetManualControl("light.a", true))

	require.NotNil(t, r.last)
	assert.Nil(t, r.last.Target, "adaptive_lighting takes its entity in service_data, not a target")
	assert.Equal(t, EntityID("light.a"), r.last.ServiceData["entity_id"])
	assert.Equal(t, true, r.last.ServiceData["manual_control"])
}

// Notify's service name is the argument, not a constant, so it must reach the
// Service field verbatim.
func TestNotifyUsesTheGivenServiceName(t *testing.T) {
	r := &recorder{}
	err := BuildService[Notify](r).Notify(types.NotifyRequest{
		ServiceName: "mobile_app_sams_iphone",
		Message:     "door open",
		Title:       "alert",
		Data:        map[string]any{"priority": "high"},
	})
	require.NoError(t, err)

	require.NotNil(t, r.last)
	assert.Equal(t, "notify", r.last.Domain)
	assert.Equal(t, "mobile_app_sams_iphone", r.last.Service)
	assert.Nil(t, r.last.Target)
	assert.Equal(t, "door open", r.last.ServiceData["message"])
	assert.Equal(t, "alert", r.last.ServiceData["title"])
	assert.Equal(t, map[string]any{"priority": "high"}, r.last.ServiceData["data"])
}

// Notify omits data when none is given rather than sending a null.
func TestNotifyOmitsAbsentData(t *testing.T) {
	r := &recorder{}
	require.NoError(t, BuildService[Notify](r).Notify(types.NotifyRequest{
		ServiceName: "persistent_notification",
		Message:     "hi",
	}))

	require.NotNil(t, r.last)
	assert.NotContains(t, r.last.ServiceData, "data")
}

// Fire produces a FireEventRequest, a different message shape from a service
// call, carrying the event type and optional data.
func TestEventFireProducesAFireEventRequest(t *testing.T) {
	r := &reqRecorder{}
	require.NoError(t, BuildService[Event](r).Fire("custom_event", map[string]any{"n": 1}))

	fe, ok := r.last.(*FireEventRequest)
	require.True(t, ok, "Fire must send a *FireEventRequest")
	assert.Equal(t, "fire_event", fe.Type)
	assert.Equal(t, "custom_event", fe.EventType)
	assert.Equal(t, map[string]any{"n": 1}, fe.EventData)
}

// The service methods that act on no entity must omit the target, which Home
// Assistant reads as malformed when sent empty.
func TestNoTargetServiceMethods(t *testing.T) {
	r := &recorder{}

	tests := []struct {
		name    string
		call    func() error
		domain  string
		service string
	}{
		{"timer reload", func() error { return BuildService[Timer](r).Reload() }, "timer", "reload"},
		{"input_text reload", func() error { return BuildService[InputText](r).Reload() }, "input_text", "reload"},
		{"input_number reload", func() error { return BuildService[InputNumber](r).Reload() }, "input_number", "reload"},
		{"input_datetime reload", func() error { return BuildService[InputDatetime](r).Reload() }, "input_datetime", "reload"},
		{"input_button reload", func() error { return BuildService[InputButton](r).Reload() }, "input_button", "reload"},
		{"input_boolean reload", func() error { return BuildService[InputBoolean](r).Reload() }, "input_boolean", "reload"},
		{"scene reload", func() error { return BuildService[Scene](r).Reload() }, "scene", "reload"},
		{"scene apply", func() error { return BuildService[Scene](r).Apply() }, "scene", "apply"},
		{"tts clear cache", func() error { return BuildService[TTS](r).ClearCache() }, "tts", "clear_cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.last = nil
			require.NoError(t, tt.call())
			require.NotNil(t, r.last)
			assert.Equal(t, tt.domain, r.last.Domain)
			assert.Equal(t, tt.service, r.last.Service)
			assert.Nil(t, r.last.Target, "a targetless call must omit the target")
		})
	}
}
