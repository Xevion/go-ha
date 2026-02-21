package connect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive a real socket, so they deliberately sit outside a synctest
// bubble. They exist to cover the coder/websocket adapter itself, which every
// other test in this package replaces with an in-memory pipe.

// realHA serves the Home Assistant handshake over an actual websocket.
func realHA(t *testing.T, token string) *url.URL {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		write := func(msg string) error {
			return conn.Write(ctx, websocket.MessageText, []byte(msg))
		}

		if err := write(`{"type":"auth_required","ha_version":"2026.2.1"}`); err != nil {
			return
		}

		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var auth struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(raw, &auth); err != nil {
			return
		}
		if auth.AccessToken != token {
			_ = write(`{"type":"auth_invalid","message":"Invalid access token"}`)
			return
		}
		if err := write(`{"type":"auth_ok","ha_version":"2026.2.1"}`); err != nil {
			return
		}

		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var req struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return
			}

			switch req.Type {
			case typePing:
				err = write(`{"id":` + itoa(req.ID) + `,"type":"pong"}`)
			case typeSubscribeEvents:
				if err := write(`{"id":` + itoa(req.ID) + `,"type":"result","success":true}`); err != nil {
					return
				}
				err = write(`{"id":` + itoa(req.ID) + `,"type":"event","event":{"event_type":"state_changed","data":{}}}`)
			default:
				err = write(`{"id":` + itoa(req.ID) + `,"type":"result","success":true}`)
			}
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	return parsed
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestWebsocketTransportRoundTrip(t *testing.T) {
	base := realHA(t, testToken)

	c, err := NewClient(base, testToken, Options{PingInterval: time.Hour})
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	defer c.Close()

	msg, err := c.Call(context.Background(), mapRequest{"type": typePing})
	require.NoError(t, err)
	assert.Equal(t, typePong, msg.Type)
}

func TestWebsocketTransportDeliversEvents(t *testing.T) {
	base := realHA(t, testToken)

	c, err := NewClient(base, testToken, Options{PingInterval: time.Hour})
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	defer c.Close()

	delivered := make(chan Message, 1)
	require.NoError(t, c.Subscribe(Subscription{EventType: "state_changed"}, func(m Message) {
		select {
		case delivered <- m:
		default:
		}
	}))

	select {
	case m := <-delivered:
		assert.Equal(t, typeEvent, m.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived over a real websocket")
	}
}

func TestWebsocketTransportRejectsBadToken(t *testing.T) {
	base := realHA(t, testToken)

	c, err := NewClient(base, "wrong-token", Options{})
	require.NoError(t, err)

	err = c.Connect(context.Background())
	assert.ErrorIs(t, err, ErrAuthFailed)
	assert.NoError(t, c.Close())
}

func TestWebsocketTransportReconnectsOverARealSocket(t *testing.T) {
	var (
		connections atomic.Int64
		accepted    = make(chan *websocket.Conn, 4)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		accepted <- conn

		ctx := r.Context()
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"auth_required"}`)); err != nil {
			return
		}
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"auth_ok"}`)); err != nil {
			return
		}
		// Hold the connection open until the client or the test closes it.
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	c, err := NewClient(base, testToken, Options{PingInterval: time.Hour})
	require.NoError(t, err)
	require.NoError(t, c.Connect(context.Background()))
	defer c.Close()

	first := <-accepted
	// Hang up the way a restarting Home Assistant would.
	first.CloseNow()

	select {
	case <-accepted:
		assert.GreaterOrEqual(t, connections.Load(), int64(2))
	case <-time.After(15 * time.Second):
		t.Fatal("client did not reconnect over a real socket")
	}
}

func TestWebsocketDialerRejectsUnknownScheme(t *testing.T) {
	_, err := NewClient(&url.URL{Scheme: "ftp", Host: "example.invalid"}, testToken, Options{})
	assert.Error(t, err)
}

func TestWebsocketDialerReportsUnreachableHost(t *testing.T) {
	// Port 0 is never listening, so this fails at dial rather than handshake.
	base := &url.URL{Scheme: "http", Host: "127.0.0.1:1"}

	c, err := NewClient(base, testToken, Options{DialTimeout: 2 * time.Second})
	require.NoError(t, err)

	err = c.Connect(context.Background())
	assert.Error(t, err)
	assert.NoError(t, c.Close())
}
