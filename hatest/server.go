// Package hatest runs an in-process Home Assistant for tests.
//
// It speaks enough of the websocket and REST APIs to stand behind a real
// ha.App: the auth handshake, event subscriptions, service calls, and the
// state endpoints. Automations can then be exercised end to end without a
// Home Assistant instance, and asserted on by what they called.
package hatest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Token is the access token the server accepts. Any other is refused, so the
// auth failure path can be exercised too.
const Token = "hatest-token"

// ServiceCall records one call an automation made.
type ServiceCall struct {
	Domain      string
	Service     string
	EntityID    string
	ServiceData map[string]any
}

// Server is an in-process Home Assistant.
type Server struct {
	t    testing.TB
	http *httptest.Server

	// handlers tracks live websocket handlers, so Close can wait for them to
	// unwind before tearing the listener down. httptest's Close otherwise sits
	// for its full five second grace period waiting on a connection that has
	// already been told to go away.
	handlers sync.WaitGroup

	mu       sync.Mutex
	closed   bool
	entities map[string]entity
	calls    []ServiceCall
	// subs maps a subscription id to the event type it wants, per connection.
	conns map[*connection]struct{}
}

type entity struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
	LastUpdated time.Time      `json:"last_updated"`
}

type connection struct {
	ws   *websocket.Conn
	mu   sync.Mutex
	subs map[int64]string
}

// New starts a server and registers its shutdown with t.
func New(t testing.TB) *Server {
	t.Helper()

	s := &Server{
		t:        t,
		entities: map[string]entity{},
		conns:    map[*connection]struct{}{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/websocket", s.serveWebsocket)
	mux.HandleFunc("/api/states/", s.serveState)
	mux.HandleFunc("/api/states", s.serveStates)

	s.http = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// URL is the address to give ha.NewAppRequest.
func (s *Server) URL() string { return s.http.URL }

// Close shuts the server down and closes any live websocket, which is what
// releases the goroutine reading it. httptest's own Close waits on idle
// connections, and a websocket never becomes idle.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true

	conns := make([]*connection, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	// CloseNow rather than Close: a graceful close waits for the peer's reply
	// frame, and a client being torn down alongside the server never sends one.
	for _, c := range conns {
		_ = c.ws.CloseNow()
	}
	s.handlers.Wait()

	s.http.Close()
}

// SetState installs an entity without announcing it, for setting up the world
// before an App connects.
func (s *Server) SetState(entityID, state string, attributes ...map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entities[entityID] = s.buildEntity(entityID, state, attributes)
}

// SetSun installs sun.sun with the given times, which is what sun triggers
// read.
func (s *Server) SetSun(up bool, nextRising, nextSetting time.Time) {
	state := "below_horizon"
	if up {
		state = "above_horizon"
	}
	// Nanosecond precision, as Home Assistant sends. Truncating to the second
	// would put a time moments away in the past.
	s.SetState("sun.sun", state, map[string]any{
		"next_rising":  nextRising.Format(time.RFC3339Nano),
		"next_setting": nextSetting.Format(time.RFC3339Nano),
		"next_dawn":    nextRising.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		"next_dusk":    nextSetting.Add(30 * time.Minute).Format(time.RFC3339Nano),
	})
}

func (s *Server) buildEntity(entityID, state string, attributes []map[string]any) entity {
	// Copied, not retained: a caller that reuses or mutates its map would
	// otherwise change state the server has already reported.
	attrs := map[string]any{}
	if len(attributes) > 0 {
		for k, v := range attributes[0] {
			attrs[k] = v
		}
	}
	now := time.Now()
	return entity{
		EntityID:    entityID,
		State:       state,
		Attributes:  attrs,
		LastChanged: now,
		LastUpdated: now,
	}
}

// ChangeState updates an entity and announces it, which is what fires
// automations watching it.
func (s *Server) ChangeState(entityID, state string, attributes ...map[string]any) {
	s.mu.Lock()
	old, existed := s.entities[entityID]
	next := s.buildEntity(entityID, state, attributes)
	s.entities[entityID] = next

	conns := make([]*connection, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	var oldState any
	if existed {
		oldState = old
	}

	s.broadcast(conns, "state_changed", map[string]any{
		"entity_id": entityID,
		"old_state": oldState,
		"new_state": next,
	})
}

// RemoveState announces an entity being deleted.
func (s *Server) RemoveState(entityID string) {
	s.mu.Lock()
	old, existed := s.entities[entityID]
	delete(s.entities, entityID)

	conns := make([]*connection, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	if !existed {
		return
	}
	s.broadcast(conns, "state_changed", map[string]any{
		"entity_id": entityID,
		"old_state": old,
		"new_state": nil,
	})
}

// Fire announces an arbitrary event, for triggers watching event types this
// package does not model.
func (s *Server) Fire(eventType string, data map[string]any) {
	s.mu.Lock()
	conns := make([]*connection, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	s.broadcast(conns, eventType, data)
}

func (s *Server) broadcast(conns []*connection, eventType string, data map[string]any) {
	payload := map[string]any{
		"event_type": eventType,
		"data":       data,
		"origin":     "LOCAL",
		"time_fired": time.Now().Format(time.RFC3339Nano),
	}

	for _, c := range conns {
		c.mu.Lock()
		ids := make([]int64, 0, len(c.subs))
		for id, want := range c.subs {
			if want == "" || want == eventType {
				ids = append(ids, id)
			}
		}
		c.mu.Unlock()

		for _, id := range ids {
			_ = c.write(map[string]any{"id": id, "type": "event", "event": payload})
		}
	}
}

// Calls returns the service calls made so far, oldest first.
func (s *Server) Calls() []ServiceCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ServiceCall(nil), s.calls...)
}

// WaitForCalls blocks until at least n service calls have been made, and
// reports them. It fails the test rather than hanging if they do not arrive.
func (s *Server) WaitForCalls(n int) []ServiceCall {
	s.t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if calls := s.Calls(); len(calls) >= n {
			return calls
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("expected %d service call(s), saw %d", n, len(s.Calls()))
			return nil
		}
		time.Sleep(time.Millisecond)
	}
}

func (s *Server) serveStates(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	list := make([]entity, 0, len(s.entities))
	for _, e := range s.entities {
		list = append(list, e)
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) serveState(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/states/"):]

	s.mu.Lock()
	e, ok := s.entities[id]
	s.mu.Unlock()

	if !ok {
		http.Error(w, `{"message":"Entity not found."}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

func (s *Server) serveWebsocket(w http.ResponseWriter, r *http.Request) {
	s.handlers.Add(1)
	defer s.handlers.Done()

	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(16 << 20)

	c := &connection{ws: ws, subs: map[int64]string{}}
	ctx := r.Context()

	// Registered before the handshake, not after. Close only shuts connections
	// it knows about, and a handler still authenticating would otherwise be
	// waited on while holding a socket nobody had closed.
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		_ = ws.CloseNow()
	}()

	if err := s.authenticate(ctx, c); err != nil {
		return
	}

	s.readLoop(ctx, c)
}

func (s *Server) authenticate(ctx context.Context, c *connection) error {
	if err := c.write(map[string]any{"type": "auth_required", "ha_version": "2026.7.0"}); err != nil {
		return err
	}

	msg, err := c.read(ctx)
	if err != nil {
		return err
	}
	if msg["type"] != "auth" || msg["access_token"] != Token {
		_ = c.write(map[string]any{"type": "auth_invalid", "message": "Invalid access token"})
		return fmt.Errorf("bad token")
	}

	return c.write(map[string]any{"type": "auth_ok", "ha_version": "2026.7.0"})
}

func (s *Server) readLoop(ctx context.Context, c *connection) {
	for {
		msg, err := c.read(ctx)
		if err != nil {
			return
		}

		id, _ := msg["id"].(float64)

		switch msg["type"] {
		case "subscribe_events":
			eventType, _ := msg["event_type"].(string)
			c.mu.Lock()
			c.subs[int64(id)] = eventType
			c.mu.Unlock()
			_ = c.write(map[string]any{"id": int64(id), "type": "result", "success": true})

		case "call_service":
			s.recordCall(msg)
			_ = c.write(map[string]any{"id": int64(id), "type": "result", "success": true})

		case "ping":
			_ = c.write(map[string]any{"id": int64(id), "type": "pong"})

		default:
			_ = c.write(map[string]any{"id": int64(id), "type": "result", "success": true})
		}
	}
}

func (s *Server) recordCall(msg map[string]any) {
	call := ServiceCall{}
	call.Domain, _ = msg["domain"].(string)
	call.Service, _ = msg["service"].(string)

	if data, ok := msg["service_data"].(map[string]any); ok {
		call.ServiceData = data
	}
	if target, ok := msg["target"].(map[string]any); ok {
		call.EntityID, _ = target["entity_id"].(string)
	}

	s.mu.Lock()
	s.calls = append(s.calls, call)
	s.mu.Unlock()
}

func (c *connection) write(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// Serialised because events are broadcast from whichever goroutine changed
	// the state, while replies come from the read loop.
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, raw)
}

func (c *connection) read(ctx context.Context) (map[string]any, error) {
	_, raw, err := c.ws.Read(ctx)
	if err != nil {
		return nil, err
	}

	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}
