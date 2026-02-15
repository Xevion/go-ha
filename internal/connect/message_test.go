package connect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMessage(t *testing.T) {
	t.Run("result carries its success flag", func(t *testing.T) {
		msg, err := parseMessage([]byte(`{"id":7,"type":"result","success":true,"result":null}`))
		require.NoError(t, err)

		assert.Equal(t, int64(7), msg.ID)
		assert.Equal(t, typeResult, msg.Type)
		assert.True(t, msg.Success)
		assert.NoError(t, msg.err())
	})

	t.Run("absent success defaults to true", func(t *testing.T) {
		// Events never carry the field, and defaulting it to false would make
		// every event look like a failed call.
		msg, err := parseMessage([]byte(`{"id":3,"type":"event","event":{"event_type":"state_changed"}}`))
		require.NoError(t, err)

		assert.True(t, msg.Success)
		assert.False(t, msg.isResult())
	})

	t.Run("failed result exposes the home assistant error", func(t *testing.T) {
		msg, err := parseMessage([]byte(
			`{"id":9,"type":"result","success":false,"error":{"code":"not_found","message":"Entity not found"}}`))
		require.NoError(t, err)

		assert.False(t, msg.Success)
		require.Error(t, msg.err())
		assert.ErrorIs(t, msg.err(), ErrCallFailed)
		assert.Contains(t, msg.err().Error(), "Entity not found")
		assert.Contains(t, msg.err().Error(), "not_found")
	})

	t.Run("failed result without an error object still fails", func(t *testing.T) {
		msg, err := parseMessage([]byte(`{"id":9,"type":"result","success":false}`))
		require.NoError(t, err)
		assert.ErrorIs(t, msg.err(), ErrCallFailed)
	})

	t.Run("pong counts as a result so it correlates by id", func(t *testing.T) {
		msg, err := parseMessage([]byte(`{"id":4,"type":"pong"}`))
		require.NoError(t, err)
		assert.True(t, msg.isResult())
	})

	t.Run("malformed json is reported, not swallowed", func(t *testing.T) {
		_, err := parseMessage([]byte(`{"id":`))
		assert.Error(t, err)
	})

	t.Run("raw payload is retained for the caller to decode", func(t *testing.T) {
		raw := []byte(`{"id":1,"type":"event","event":{"event_type":"call_service"}}`)
		msg, err := parseMessage(raw)
		require.NoError(t, err)
		assert.Equal(t, raw, msg.Raw)
	})
}
