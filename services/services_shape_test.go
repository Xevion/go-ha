package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/types"
)

// recorder captures the request a service method produced.
type recorder struct{ last *BaseServiceRequest }

func (r *recorder) Send(req types.Request) error {
	r.last = req.(*BaseServiceRequest)
	return nil
}

// Every service method is a handful of near-identical assignments written by
// hand, which is exactly where a copy-paste sends turn_off to the wrong domain
// or repeats the previous method's service name.
func TestServiceMethodsAddressTheRightService(t *testing.T) {
	r := &recorder{}

	tests := []struct {
		name    string
		call    func() error
		domain  string
		service string
		entity  string
	}{
		{"light on", func() error { return BuildService[Light](r).TurnOn("light.a") }, "light", "turn_on", "light.a"},
		{"light off", func() error { return BuildService[Light](r).TurnOff("light.a") }, "light", "turn_off", "light.a"},
		{"light toggle", func() error { return BuildService[Light](r).Toggle("light.a") }, "light", "toggle", "light.a"},

		{"switch on", func() error { return BuildService[Switch](r).TurnOn("switch.a") }, "switch", "turn_on", "switch.a"},
		{"switch off", func() error { return BuildService[Switch](r).TurnOff("switch.a") }, "switch", "turn_off", "switch.a"},

		{"cover open", func() error { return BuildService[Cover](r).Open("cover.a") }, "cover", "open_cover", "cover.a"},
		{"cover close", func() error { return BuildService[Cover](r).Close("cover.a") }, "cover", "close_cover", "cover.a"},
		{"cover stop", func() error { return BuildService[Cover](r).Stop("cover.a") }, "cover", "stop_cover", "cover.a"},
		{"cover open tilt", func() error { return BuildService[Cover](r).OpenTilt("cover.a") }, "cover", "open_cover_tilt", "cover.a"},
		{"cover close tilt", func() error { return BuildService[Cover](r).CloseTilt("cover.a") }, "cover", "close_cover_tilt", "cover.a"},

		{"lock", func() error { return BuildService[Lock](r).Lock("lock.a") }, "lock", "lock", "lock.a"},
		{"unlock", func() error { return BuildService[Lock](r).Unlock("lock.a") }, "lock", "unlock", "lock.a"},

		{"media play", func() error { return BuildService[MediaPlayer](r).Play("media_player.a") }, "media_player", "media_play", "media_player.a"},
		{"media pause", func() error { return BuildService[MediaPlayer](r).Pause("media_player.a") }, "media_player", "media_pause", "media_player.a"},
		{"media next", func() error { return BuildService[MediaPlayer](r).Next("media_player.a") }, "media_player", "media_next_track", "media_player.a"},

		{"input_boolean on", func() error { return BuildService[InputBoolean](r).TurnOn("input_boolean.a") }, "input_boolean", "turn_on", "input_boolean.a"},
		{"input_button press", func() error { return BuildService[InputButton](r).Press("input_button.a") }, "input_button", "press", "input_button.a"},

		{"scene on", func() error { return BuildService[Scene](r).TurnOn("scene.a") }, "scene", "turn_on", "scene.a"},
		{"script on", func() error { return BuildService[Script](r).TurnOn("script.a") }, "script", "turn_on", "script.a"},

		{"alarm disarm", func() error { return BuildService[AlarmControlPanel](r).Disarm("alarm_control_panel.a") }, "alarm_control_panel", "alarm_disarm", "alarm_control_panel.a"},
		{"alarm arm home", func() error { return BuildService[AlarmControlPanel](r).ArmHome("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_home", "alarm_control_panel.a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.last = nil
			require.NoError(t, tt.call())
			require.NotNil(t, r.last)

			assert.Equal(t, tt.domain, r.last.Domain)
			assert.Equal(t, tt.service, r.last.Service)
			assert.Equal(t, "call_service", r.last.RequestType)

			require.NotNil(t, r.last.Target, "an entity-scoped call must carry a target")
			assert.Equal(t, tt.entity, r.last.Target.EntityId)
		})
	}
}

// A call naming no entity must omit the target rather than sending an empty
// one, which Home Assistant reads as a malformed request.
func TestCallWithoutAnEntityOmitsTheTarget(t *testing.T) {
	req := NewBaseServiceRequest("")
	assert.Nil(t, req.Target)
}

func TestServiceDataIsCarried(t *testing.T) {
	r := &recorder{}

	require.NoError(t, BuildService[Light](r).TurnOn("light.a", map[string]any{"brightness": 200}))
	assert.Equal(t, 200, r.last.ServiceData["brightness"])
}

// Home Assistant gains services faster than this package models them, and
// custom integrations define their own. Call is the escape hatch.
func TestCallReachesAnUnmodelledService(t *testing.T) {
	r := &recorder{}

	require.NoError(t, Call(r, "custom_integration", "do_thing", "light.a",
		map[string]any{"mode": "fast"}))

	require.NotNil(t, r.last)
	assert.Equal(t, "custom_integration", r.last.Domain)
	assert.Equal(t, "do_thing", r.last.Service)
	assert.Equal(t, "light.a", r.last.Target.EntityId)
	assert.Equal(t, "fast", r.last.ServiceData["mode"])
}

// Zero degrees is an ordinary setpoint in Celsius. Treating it as "unset"
// dropped it from the call and left the thermostat where it was.
func TestSetTemperatureSendsAZeroSetpoint(t *testing.T) {
	req := types.SetTemperatureRequest{Temperature: types.Ptr(float32(0))}

	payload := req.ToJSON()
	require.Contains(t, payload, "temperature")
	assert.Equal(t, float32(0), payload["temperature"])
}

func TestSetTemperatureOmitsUnsetFields(t *testing.T) {
	req := types.SetTemperatureRequest{Temperature: types.Ptr(float32(21))}

	payload := req.ToJSON()
	assert.NotContains(t, payload, "target_temp_high")
	assert.NotContains(t, payload, "target_temp_low")
	assert.NotContains(t, payload, "hvac_mode")
}

func TestCallWithoutATargetOmitsIt(t *testing.T) {
	r := &recorder{}

	require.NoError(t, Call(r, "homeassistant", "restart", "", nil))
	assert.Nil(t, r.last.Target)
}
