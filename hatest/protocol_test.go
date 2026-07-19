package hatest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// handshake dials the server and reads the auth_required greeting, leaving the
// connection ready to authenticate.
func handshake(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.Dial(t.Context(), s.URL()+"/api/websocket", nil)
	require.NoError(t, err)

	greeting := readMsg(t, ws)
	require.Equal(t, "auth_required", greeting["type"])
	return ws
}

func readMsg(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	_, raw, err := ws.Read(t.Context())
	require.NoError(t, err)
	var msg map[string]any
	require.NoError(t, json.Unmarshal(raw, &msg))
	return msg
}

func writeMsg(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, ws.Write(t.Context(), websocket.MessageText, raw))
}

// A wrong token must be refused, so the auth failure path a real client would
// hit is reachable in tests too.
func TestAuthInvalidRejectsABadToken(t *testing.T) {
	s := New(t)
	ws := handshake(t, s)
	defer ws.CloseNow()

	writeMsg(t, ws, map[string]any{"type": "auth", "access_token": "wrong"})

	reply := readMsg(t, ws)
	assert.Equal(t, "auth_invalid", reply["type"])
}

func TestAuthOkAcceptsTheToken(t *testing.T) {
	s := New(t)
	ws := handshake(t, s)
	defer ws.CloseNow()

	writeMsg(t, ws, map[string]any{"type": "auth", "access_token": Token})

	reply := readMsg(t, ws)
	assert.Equal(t, "auth_ok", reply["type"])
}

// The client's keepalive ping must be answered, or a real connection would be
// judged dead.
func TestPingIsAnsweredWithPong(t *testing.T) {
	s := New(t)
	ws := handshake(t, s)
	defer ws.CloseNow()

	writeMsg(t, ws, map[string]any{"type": "auth", "access_token": Token})
	require.Equal(t, "auth_ok", readMsg(t, ws)["type"])

	writeMsg(t, ws, map[string]any{"id": 5, "type": "ping"})

	reply := readMsg(t, ws)
	assert.Equal(t, "pong", reply["type"])
	assert.Equal(t, float64(5), reply["id"])
}

func TestSubscribeEventsIsAcknowledged(t *testing.T) {
	s := New(t)
	ws := handshake(t, s)
	defer ws.CloseNow()

	writeMsg(t, ws, map[string]any{"type": "auth", "access_token": Token})
	require.Equal(t, "auth_ok", readMsg(t, ws)["type"])

	writeMsg(t, ws, map[string]any{"id": 1, "type": "subscribe_events", "event_type": "state_changed"})

	reply := readMsg(t, ws)
	assert.Equal(t, "result", reply["type"])
	assert.Equal(t, true, reply["success"])
}

func TestServeStateReturns404ForAnUnknownEntity(t *testing.T) {
	s := New(t)
	resp, err := http.Get(s.URL() + "/api/states/light.nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServeStateReturnsASeededEntity(t *testing.T) {
	s := New(t)
	s.SetState("light.hall", "on", map[string]any{"brightness": 200})

	resp, err := http.Get(s.URL() + "/api/states/light.hall")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, "on", got["state"])
	assert.Equal(t, "light.hall", got["entity_id"])
}
