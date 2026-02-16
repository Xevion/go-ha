package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
)

// fakeHA is an in-memory Home Assistant speaking the websocket protocol over
// channels. It exists because a real socket is not durably blocking, so a test
// using one would stop synctest's clock and turn every backoff into a real
// sleep.
type fakeHA struct {
	t     *testing.T
	token string

	mu       sync.Mutex
	conns    []*fakeConn
	dials    int
	dialErr  error
	refuseAt int // dial attempt number from which dialErr applies, 1-based
}

func newFakeHA(t *testing.T, token string) *fakeHA {
	return &fakeHA{t: t, token: token}
}

// failDialsFrom makes every dial from attempt n onwards fail with err.
func (h *fakeHA) failDialsFrom(n int, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refuseAt = n
	h.dialErr = err
}

// allowDials clears any configured dial failure.
func (h *fakeHA) allowDials() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.refuseAt = 0
	h.dialErr = nil
}

func (h *fakeHA) dialCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dials
}

// current returns the most recently accepted connection.
func (h *fakeHA) current() *fakeConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.conns) == 0 {
		return nil
	}
	return h.conns[len(h.conns)-1]
}

func (h *fakeHA) dial(ctx context.Context) (transport, error) {
	h.mu.Lock()
	h.dials++
	attempt := h.dials
	if h.refuseAt > 0 && attempt >= h.refuseAt && h.dialErr != nil {
		err := h.dialErr
		h.mu.Unlock()
		return nil, err
	}

	conn := &fakeConn{
		toClient:   make(chan []byte, 64),
		fromClient: make(chan []byte, 64),
		closed:     make(chan struct{}),
	}
	h.conns = append(h.conns, conn)
	h.mu.Unlock()

	go h.serve(conn)
	return conn, nil
}

// serve runs the server side of one connection.
func (h *fakeHA) serve(conn *fakeConn) {
	conn.push(`{"type":"auth_required","ha_version":"2026.2.1"}`)

	raw, ok := conn.pull()
	if !ok {
		return
	}
	var auth struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &auth); err != nil || auth.Type != typeAuth {
		conn.push(`{"type":"auth_invalid","message":"malformed auth"}`)
		conn.serverClose()
		return
	}
	if auth.AccessToken != h.token {
		conn.push(`{"type":"auth_invalid","message":"Invalid access token"}`)
		conn.serverClose()
		return
	}
	conn.push(`{"type":"auth_ok","ha_version":"2026.2.1"}`)

	for {
		raw, ok := conn.pull()
		if !ok {
			return
		}

		var req struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}
		conn.record(req.Type, req.ID)

		switch req.Type {
		case typePing:
			if conn.swallowPings() {
				continue
			}
			conn.pushf(`{"id":%d,"type":"pong"}`, req.ID)
		case typeSubscribeEvents:
			conn.pushf(`{"id":%d,"type":"result","success":true,"result":null}`, req.ID)
		default:
			conn.pushf(`{"id":%d,"type":"result","success":true,"result":null}`, req.ID)
		}
	}
}

// fakeConn is one in-memory connection, implementing transport.
type fakeConn struct {
	toClient   chan []byte
	fromClient chan []byte

	closed    chan struct{}
	closeOnce sync.Once

	mu           sync.Mutex
	seen         []seenRequest
	ignorePings  bool
	subscribeIDs []int64
}

type seenRequest struct {
	Type string
	ID   int64
}

func (c *fakeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case data := <-c.toClient:
		return data, nil
	case <-c.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *fakeConn) Write(ctx context.Context, data []byte) error {
	// Copy: the client reuses nothing today, but a test that inspects messages
	// later should not be reading a buffer someone else may still own.
	buf := make([]byte, len(data))
	copy(buf, data)

	select {
	case c.fromClient <- buf:
		return nil
	case <-c.closed:
		return errors.New("write on closed fake connection")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// serverClose drops the connection from the server side, as Home Assistant
// does when it hangs up on a backed-up client.
func (c *fakeConn) serverClose() { _ = c.Close() }

func (c *fakeConn) push(msg string) { c.pushRaw([]byte(msg)) }

func (c *fakeConn) pushf(format string, args ...any) {
	c.pushRaw(fmt.Appendf(nil, format, args...))
}

func (c *fakeConn) pushRaw(data []byte) {
	select {
	case c.toClient <- data:
	case <-c.closed:
	}
}

func (c *fakeConn) pull() ([]byte, bool) {
	select {
	case data := <-c.fromClient:
		return data, true
	case <-c.closed:
		return nil, false
	}
}

func (c *fakeConn) record(msgType string, id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, seenRequest{Type: msgType, ID: id})
	if msgType == typeSubscribeEvents {
		c.subscribeIDs = append(c.subscribeIDs, id)
	}
}

// requests returns every message the server received on this connection.
func (c *fakeConn) requests() []seenRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]seenRequest(nil), c.seen...)
}

// countOf reports how many messages of a type this connection received.
func (c *fakeConn) countOf(msgType string) int {
	n := 0
	for _, r := range c.requests() {
		if r.Type == msgType {
			n++
		}
	}
	return n
}

// subscriptions returns the ids of the subscribe_events requests received.
func (c *fakeConn) subscriptions() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.subscribeIDs...)
}

// ignorePingsFrom makes the server stop answering pings, simulating a wedged
// event loop behind a socket that still accepts bytes.
func (c *fakeConn) ignorePingsFrom() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ignorePings = true
}

func (c *fakeConn) swallowPings() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ignorePings
}

// emit sends an event on the given subscription id.
func (c *fakeConn) emit(subID int64, eventType string) {
	c.pushf(`{"id":%d,"type":"event","event":{"event_type":%q,"data":{}}}`, subID, eventType)
}
