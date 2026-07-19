package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetTemperatureToJSON(t *testing.T) {
	tests := []struct {
		name string
		req  SetTemperatureRequest
		want map[string]any
	}{
		{
			"empty request sends nothing",
			SetTemperatureRequest{},
			map[string]any{},
		},
		{
			"a zero setpoint is a real value, not absent",
			SetTemperatureRequest{Temperature: Ptr(float32(0))},
			map[string]any{"temperature": float32(0)},
		},
		{
			"single setpoint",
			SetTemperatureRequest{Temperature: Ptr(float32(21.5))},
			map[string]any{"temperature": float32(21.5)},
		},
		{
			"a range omits the single setpoint",
			SetTemperatureRequest{TargetTempHigh: Ptr(float32(24)), TargetTempLow: Ptr(float32(19))},
			map[string]any{"target_temp_high": float32(24), "target_temp_low": float32(19)},
		},
		{
			"hvac mode rides along",
			SetTemperatureRequest{Temperature: Ptr(float32(20)), HvacMode: "heat"},
			map[string]any{"temperature": float32(20), "hvac_mode": "heat"},
		},
		{
			"every field set",
			SetTemperatureRequest{
				Temperature:    Ptr(float32(21)),
				TargetTempHigh: Ptr(float32(24)),
				TargetTempLow:  Ptr(float32(19)),
				HvacMode:       "auto",
			},
			map[string]any{
				"temperature":      float32(21),
				"target_temp_high": float32(24),
				"target_temp_low":  float32(19),
				"hvac_mode":        "auto",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.req.ToJSON())
		})
	}
}

func TestPtr(t *testing.T) {
	p := Ptr(42)
	if assert.NotNil(t, p) {
		assert.Equal(t, 42, *p)
	}

	// Distinct calls must not share storage.
	a, b := Ptr("x"), Ptr("y")
	assert.NotSame(t, a, b)
}
