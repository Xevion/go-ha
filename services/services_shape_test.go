package services

import (
	"testing"
	"time"

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

	ts := time.Unix(1700000000, 0)

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
		{"switch toggle", func() error { return BuildService[Switch](r).Toggle("switch.a") }, "switch", "toggle", "switch.a"},
		{"switch off", func() error { return BuildService[Switch](r).TurnOff("switch.a") }, "switch", "turn_off", "switch.a"},

		{"cover open", func() error { return BuildService[Cover](r).Open("cover.a") }, "cover", "open_cover", "cover.a"},
		{"cover close", func() error { return BuildService[Cover](r).Close("cover.a") }, "cover", "close_cover", "cover.a"},
		{"cover stop", func() error { return BuildService[Cover](r).Stop("cover.a") }, "cover", "stop_cover", "cover.a"},
		{"cover open tilt", func() error { return BuildService[Cover](r).OpenTilt("cover.a") }, "cover", "open_cover_tilt", "cover.a"},
		{"cover close tilt", func() error { return BuildService[Cover](r).CloseTilt("cover.a") }, "cover", "close_cover_tilt", "cover.a"},
		{"cover stop tilt", func() error { return BuildService[Cover](r).StopTilt("cover.a") }, "cover", "stop_cover_tilt", "cover.a"},
		{"cover set position", func() error { return BuildService[Cover](r).SetPosition("cover.a") }, "cover", "set_cover_position", "cover.a"},
		{"cover set tilt position", func() error { return BuildService[Cover](r).SetTiltPosition("cover.a") }, "cover", "set_cover_tilt_position", "cover.a"},
		{"cover toggle", func() error { return BuildService[Cover](r).Toggle("cover.a") }, "cover", "toggle", "cover.a"},
		{"cover toggle tilt", func() error { return BuildService[Cover](r).ToggleTilt("cover.a") }, "cover", "toggle_cover_tilt", "cover.a"},

		{"lock", func() error { return BuildService[Lock](r).Lock("lock.a") }, "lock", "lock", "lock.a"},
		{"unlock", func() error { return BuildService[Lock](r).Unlock("lock.a") }, "lock", "unlock", "lock.a"},

		{"media clear playlist", func() error { return BuildService[MediaPlayer](r).ClearPlaylist("media_player.a") }, "media_player", "clear_playlist", "media_player.a"},
		{"media join", func() error { return BuildService[MediaPlayer](r).Join("media_player.a") }, "media_player", "join", "media_player.a"},
		{"media next", func() error { return BuildService[MediaPlayer](r).Next("media_player.a") }, "media_player", "media_next_track", "media_player.a"},
		{"media pause", func() error { return BuildService[MediaPlayer](r).Pause("media_player.a") }, "media_player", "media_pause", "media_player.a"},
		{"media play", func() error { return BuildService[MediaPlayer](r).Play("media_player.a") }, "media_player", "media_play", "media_player.a"},
		{"media play pause", func() error { return BuildService[MediaPlayer](r).PlayPause("media_player.a") }, "media_player", "media_play_pause", "media_player.a"},
		{"media previous", func() error { return BuildService[MediaPlayer](r).Previous("media_player.a") }, "media_player", "media_previous_track", "media_player.a"},
		{"media seek", func() error { return BuildService[MediaPlayer](r).Seek("media_player.a") }, "media_player", "media_seek", "media_player.a"},
		{"media stop", func() error { return BuildService[MediaPlayer](r).Stop("media_player.a") }, "media_player", "media_stop", "media_player.a"},
		{"media play media", func() error { return BuildService[MediaPlayer](r).PlayMedia("media_player.a") }, "media_player", "play_media", "media_player.a"},
		{"media repeat set", func() error { return BuildService[MediaPlayer](r).RepeatSet("media_player.a") }, "media_player", "repeat_set", "media_player.a"},
		{"media select sound mode", func() error { return BuildService[MediaPlayer](r).SelectSoundMode("media_player.a") }, "media_player", "select_sound_mode", "media_player.a"},
		{"media select source", func() error { return BuildService[MediaPlayer](r).SelectSource("media_player.a") }, "media_player", "select_source", "media_player.a"},
		{"media shuffle", func() error { return BuildService[MediaPlayer](r).Shuffle("media_player.a") }, "media_player", "shuffle_set", "media_player.a"},
		{"media toggle", func() error { return BuildService[MediaPlayer](r).Toggle("media_player.a") }, "media_player", "toggle", "media_player.a"},
		{"media turn off", func() error { return BuildService[MediaPlayer](r).TurnOff("media_player.a") }, "media_player", "turn_off", "media_player.a"},
		{"media turn on", func() error { return BuildService[MediaPlayer](r).TurnOn("media_player.a") }, "media_player", "turn_on", "media_player.a"},
		{"media unjoin", func() error { return BuildService[MediaPlayer](r).Unjoin("media_player.a") }, "media_player", "unjoin", "media_player.a"},
		{"media volume down", func() error { return BuildService[MediaPlayer](r).VolumeDown("media_player.a") }, "media_player", "volume_down", "media_player.a"},
		{"media volume mute", func() error { return BuildService[MediaPlayer](r).VolumeMute("media_player.a") }, "media_player", "volume_mute", "media_player.a"},
		{"media volume set", func() error { return BuildService[MediaPlayer](r).VolumeSet("media_player.a") }, "media_player", "volume_set", "media_player.a"},
		{"media volume up", func() error { return BuildService[MediaPlayer](r).VolumeUp("media_player.a") }, "media_player", "volume_up", "media_player.a"},

		{"input_boolean on", func() error { return BuildService[InputBoolean](r).TurnOn("input_boolean.a") }, "input_boolean", "turn_on", "input_boolean.a"},
		{"input_boolean toggle", func() error { return BuildService[InputBoolean](r).Toggle("input_boolean.a") }, "input_boolean", "toggle", "input_boolean.a"},
		{"input_boolean off", func() error { return BuildService[InputBoolean](r).TurnOff("input_boolean.a") }, "input_boolean", "turn_off", "input_boolean.a"},

		{"input_button press", func() error { return BuildService[InputButton](r).Press("input_button.a") }, "input_button", "press", "input_button.a"},

		{"input_number set", func() error { return BuildService[InputNumber](r).Set("input_number.a", 5) }, "input_number", "set_value", "input_number.a"},
		{"input_number increment", func() error { return BuildService[InputNumber](r).Increment("input_number.a") }, "input_number", "increment", "input_number.a"},
		{"input_number decrement", func() error { return BuildService[InputNumber](r).Decrement("input_number.a") }, "input_number", "decrement", "input_number.a"},

		{"input_text set", func() error { return BuildService[InputText](r).Set("input_text.a", "x") }, "input_text", "set_value", "input_text.a"},
		{"input_datetime set", func() error { return BuildService[InputDatetime](r).Set("input_datetime.a", ts) }, "input_datetime", "set_datetime", "input_datetime.a"},
		{"number set value", func() error { return BuildService[Number](r).SetValue("number.a", 5) }, "number", "set_value", "number.a"},

		{"scene on", func() error { return BuildService[Scene](r).TurnOn("scene.a") }, "scene", "turn_on", "scene.a"},
		{"scene create", func() error { return BuildService[Scene](r).Create("scene.a") }, "scene", "create", "scene.a"},

		{"script reload", func() error { return BuildService[Script](r).Reload("script.a") }, "script", "reload", "script.a"},
		{"script toggle", func() error { return BuildService[Script](r).Toggle("script.a") }, "script", "toggle", "script.a"},
		{"script off", func() error { return BuildService[Script](r).TurnOff("script.a") }, "script", "turn_off", "script.a"},
		{"script on", func() error { return BuildService[Script](r).TurnOn("script.a") }, "script", "turn_on", "script.a"},

		{"alarm arm away", func() error { return BuildService[AlarmControlPanel](r).ArmAway("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_away", "alarm_control_panel.a"},
		{"alarm arm custom bypass", func() error { return BuildService[AlarmControlPanel](r).ArmWithCustomBypass("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_custom_bypass", "alarm_control_panel.a"},
		{"alarm arm home", func() error { return BuildService[AlarmControlPanel](r).ArmHome("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_home", "alarm_control_panel.a"},
		{"alarm arm night", func() error { return BuildService[AlarmControlPanel](r).ArmNight("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_night", "alarm_control_panel.a"},
		{"alarm arm vacation", func() error { return BuildService[AlarmControlPanel](r).ArmVacation("alarm_control_panel.a") }, "alarm_control_panel", "alarm_arm_vacation", "alarm_control_panel.a"},
		{"alarm disarm", func() error { return BuildService[AlarmControlPanel](r).Disarm("alarm_control_panel.a") }, "alarm_control_panel", "alarm_disarm", "alarm_control_panel.a"},
		{"alarm trigger", func() error { return BuildService[AlarmControlPanel](r).Trigger("alarm_control_panel.a") }, "alarm_control_panel", "alarm_trigger", "alarm_control_panel.a"},

		{"vacuum clean spot", func() error { return BuildService[Vacuum](r).CleanSpot("vacuum.a") }, "vacuum", "clean_spot", "vacuum.a"},
		{"vacuum locate", func() error { return BuildService[Vacuum](r).Locate("vacuum.a") }, "vacuum", "locate", "vacuum.a"},
		{"vacuum pause", func() error { return BuildService[Vacuum](r).Pause("vacuum.a") }, "vacuum", "pause", "vacuum.a"},
		{"vacuum return to base", func() error { return BuildService[Vacuum](r).ReturnToBase("vacuum.a") }, "vacuum", "return_to_base", "vacuum.a"},
		{"vacuum send command", func() error { return BuildService[Vacuum](r).SendCommand("vacuum.a") }, "vacuum", "send_command", "vacuum.a"},
		{"vacuum set fan speed", func() error { return BuildService[Vacuum](r).SetFanSpeed("vacuum.a") }, "vacuum", "set_fan_speed", "vacuum.a"},
		{"vacuum start", func() error { return BuildService[Vacuum](r).Start("vacuum.a") }, "vacuum", "start", "vacuum.a"},
		{"vacuum start pause", func() error { return BuildService[Vacuum](r).StartPause("vacuum.a") }, "vacuum", "start_pause", "vacuum.a"},
		{"vacuum stop", func() error { return BuildService[Vacuum](r).Stop("vacuum.a") }, "vacuum", "stop", "vacuum.a"},
		{"vacuum turn off", func() error { return BuildService[Vacuum](r).TurnOff("vacuum.a") }, "vacuum", "turn_off", "vacuum.a"},
		{"vacuum turn on", func() error { return BuildService[Vacuum](r).TurnOn("vacuum.a") }, "vacuum", "turn_on", "vacuum.a"},

		{"timer start", func() error { return BuildService[Timer](r).Start("timer.a", "00:01:00") }, "timer", "start", "timer.a"},
		{"timer change", func() error { return BuildService[Timer](r).Change("timer.a", "00:01:00") }, "timer", "change", "timer.a"},
		{"timer pause", func() error { return BuildService[Timer](r).Pause("timer.a") }, "timer", "pause", "timer.a"},
		{"timer cancel", func() error { return BuildService[Timer](r).Cancel("timer.a") }, "timer", "cancel", "timer.a"},
		{"timer finish", func() error { return BuildService[Timer](r).Finish("timer.a") }, "timer", "finish", "timer.a"},

		{"climate set fan mode", func() error { return BuildService[Climate](r).SetFanMode("climate.a", "auto") }, "climate", "set_fan_mode", "climate.a"},
		{"climate set temperature", func() error {
			return BuildService[Climate](r).SetTemperature("climate.a", types.SetTemperatureRequest{Temperature: types.Ptr(float32(21))})
		}, "climate", "set_temperature", "climate.a"},

		{"tts cloud say", func() error { return BuildService[TTS](r).CloudSay("media_player.a") }, "tts", "cloud_say", "media_player.a"},
		{"tts google say", func() error { return BuildService[TTS](r).GoogleTranslateSay("media_player.a") }, "tts", "google_translate_say", "media_player.a"},

		{"zwavejs bulk set", func() error { return BuildService[ZWaveJS](r).BulkSetPartialConfigParam("sensor.a", 1, 2) }, "zwave_js", "bulk_set_partial_config_parameters", "sensor.a"},

		{"homeassistant on", func() error { return BuildService[HomeAssistant](r).TurnOn("light.a") }, "homeassistant", "turn_on", "light.a"},
		{"homeassistant toggle", func() error { return BuildService[HomeAssistant](r).Toggle("light.a") }, "homeassistant", "toggle", "light.a"},
		{"homeassistant off", func() error { return BuildService[HomeAssistant](r).TurnOff("light.a") }, "homeassistant", "turn_off", "light.a"},
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
