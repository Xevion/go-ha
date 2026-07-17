package services

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBaseServiceRequestLeavesTheIdUnset(t *testing.T) {
	req := NewBaseServiceRequest("light.kitchen")

	// The id belongs to the connection that carries the request, not to the
	// request itself, so building one must not allocate an id.
	assert.Zero(t, req.Id)
	assert.Equal(t, "call_service", req.RequestType)
	require.NotNil(t, req.Target)
	assert.Equal(t, "light.kitchen", req.Target.EntityId)
}

func TestNewBaseServiceRequestOmitsAnEmptyTarget(t *testing.T) {
	req := NewBaseServiceRequest("")
	req.Domain = "homeassistant"
	req.Service = "restart"
	req.SetID(1)

	raw, err := json.Marshal(&req)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	// omitempty does nothing for a struct value, so this used to serialise as
	// "target":{} on every call that names no entity.
	_, present := got["target"]
	assert.False(t, present, "an absent target must be omitted, not sent empty")
}

func TestSetIDStampsTheRequest(t *testing.T) {
	req := NewBaseServiceRequest("light.kitchen")
	req.SetID(42)
	assert.Equal(t, int64(42), req.Id)

	// Re-stamping has to overwrite: a request replayed on a new connection
	// carries an id the old connection issued, which means nothing there.
	req.SetID(7)
	assert.Equal(t, int64(7), req.Id)
}

func TestServiceRequestMarshalsToTheProtocolShape(t *testing.T) {
	req := NewBaseServiceRequest("light.kitchen")
	req.Domain = "light"
	req.Service = "turn_on"
	req.ServiceData = map[string]any{"brightness": 255}
	req.SetID(3)

	raw, err := json.Marshal(&req)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, float64(3), got["id"])
	assert.Equal(t, "call_service", got["type"])
	assert.Equal(t, "light", got["domain"])
	assert.Equal(t, "turn_on", got["service"])
	assert.Equal(t, map[string]any{"brightness": float64(255)}, got["service_data"])
	assert.Equal(t, map[string]any{"entity_id": "light.kitchen"}, got["target"])
}

func TestServiceRequestOmitsAbsentServiceData(t *testing.T) {
	req := NewBaseServiceRequest("switch.porch")
	req.Domain = "switch"
	req.Service = "turn_off"
	req.SetID(1)

	raw, err := json.Marshal(&req)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	_, present := got["service_data"]
	assert.False(t, present, "an empty service_data must not be sent")
}

func TestFireEventRequestStampsItsId(t *testing.T) {
	req := FireEventRequest{Type: "fire_event", EventType: "custom_event"}
	req.SetID(11)

	raw, err := json.Marshal(&req)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, float64(11), got["id"])
	assert.Equal(t, "fire_event", got["type"])
	assert.Equal(t, "custom_event", got["event_type"])
}
