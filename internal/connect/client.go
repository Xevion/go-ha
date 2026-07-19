package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// Options tunes the connection layer. The zero value is not usable; start from
// DefaultOptions and adjust.
type Options struct {
	// QueueSize bounds the event backlog held between the reader and the
	// workers. Home Assistant disconnects a client that stops draining its
	// socket for five seconds, so this queue is deliberately finite: shedding
	// load is survivable, being disconnected is not.
	QueueSize int

	// Workers is the number of goroutines draining the queue. Handlers run on
	// these, so a slow handler costs a worker rather than the connection.
	Workers int

	// PingInterval is how often liveness is checked once the connection is idle.
	PingInterval time.Duration

	// PingTimeout bounds how long a ping waits before the connection is
	// considered dead and torn down.
	PingTimeout time.Duration

	// DialTimeout bounds a single connection attempt, including the auth
	// handshake.
	DialTimeout time.Duration

	// WriteTimeout bounds a single outgoing message.
	WriteTimeout time.Duration

	// HealthyAfter is how long a connection must survive before the backoff
	// sequence resets. Without it a connection that dies immediately after each
	// handshake would retry at the base delay forever.
	HealthyAfter time.Duration

	// OnConnected runs after a connection is established and its subscriptions
	// replayed, including after a reconnect. It runs on its own goroutine: the
	// reader cannot wait on it without backing the socket up.
	OnConnected func()

	// OnEvent, if set, is called for every event message in the order it
	// arrives off the wire, on the reader goroutine, before the message is
	// queued for its handler. Handlers run concurrently on the worker pool, so
	// their order is not the wire order; this hook is where ordered state, such
	// as a cache the handlers read, is maintained. It must not block, for the
	// same reason the reader must not: a stalled reader stops draining the
	// socket and Home Assistant hangs up.
	OnEvent func(Message)
}

// DefaultOptions returns the settings used when none are supplied.
func DefaultOptions() Options {
	return Options{
		QueueSize:    256,
		Workers:      4,
		PingInterval: 30 * time.Second,
		PingTimeout:  10 * time.Second,
		DialTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		HealthyAfter: 60 * time.Second,
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.QueueSize <= 0 {
		o.QueueSize = d.QueueSize
	}
	if o.Workers <= 0 {
		o.Workers = d.Workers
	}
	if o.PingInterval <= 0 {
		o.PingInterval = d.PingInterval
	}
	if o.PingTimeout <= 0 {
		o.PingTimeout = d.PingTimeout
	}
	if o.DialTimeout <= 0 {
		o.DialTimeout = d.DialTimeout
	}
	if o.WriteTimeout <= 0 {
		o.WriteTimeout = d.WriteTimeout
	}
	if o.HealthyAfter <= 0 {
		o.HealthyAfter = d.HealthyAfter
	}
	return o
}

// Client owns a single Home Assistant websocket connection, re-establishing it
// as needed and replaying subscriptions each time it does.
type Client struct {
	dial  dialer
	token string
	opts  Options

	ctx    context.Context
	cancel context.CancelFunc

	nextID  atomic.Int64
	backoff *backoff

	// writeMu serialises id allocation with the write that carries it. Home
	// Assistant refuses any id that does not exceed the last it received, so
	// the two steps cannot be allowed to interleave across goroutines.
	//
	// Lock ordering is always writeMu before mu, never the reverse.
	writeMu sync.Mutex

	mu      sync.Mutex
	conn    transport
	pending map[int64]func(Message)
	// routes maps the ids handed out by the current connection to the
	// subscription that owns them. It is discarded on every disconnect.
	routes map[int64]*subscription
	// subs is the declarative set, and is what makes a reconnect recoverable.
	subs []*subscription
	// gen identifies the current connection. It starts at one so a freshly
	// built subscription, whose gen is zero, never looks already established.
	gen uint64

	events  chan Message
	dropped atomic.Uint64

	wg sync.WaitGroup
}

// NewClient prepares a client for the Home Assistant instance at baseUrl. No
// connection is made until Connect is called.
func NewClient(baseUrl *url.URL, token string, opts Options) (*Client, error) {
	dial, err := websocketDialer(baseUrl)
	if err != nil {
		return nil, err
	}
	return newClientWithDialer(dial, token, opts), nil
}

func newClientWithDialer(dial dialer, token string, opts Options) *Client {
	opts = opts.withDefaults()
	return &Client{
		dial:    dial,
		token:   token,
		opts:    opts,
		backoff: newBackoff(rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))),
		pending: map[int64]func(Message){},
		routes:  map[int64]*subscription{},
		events:  make(chan Message, opts.QueueSize),
		gen:     1,
	}
}

// setConn installs a new connection and advances the generation, invalidating
// every id the previous one handed out.
func (c *Client) setConn(conn transport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
	c.gen++
}

// Connect establishes the first connection and starts the goroutines that keep
// it alive. It fails fast, so an unreachable host or a refused token surfaces
// to the caller rather than disappearing into a retry loop.
func (c *Client) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	conn, err := c.connectOnce(c.ctx)
	if err != nil {
		c.cancel()
		return err
	}

	c.setConn(conn)

	for range c.opts.Workers {
		c.wg.Add(1)
		go c.worker()
	}

	c.wg.Add(1)
	go c.run(conn)

	return nil
}

// Close shuts the client down and waits for its goroutines to finish.
func (c *Client) Close() error {
	if c.cancel == nil {
		return nil
	}
	c.cancel()

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		// Unblocks the reader, which is otherwise parked on a socket that will
		// not produce another message.
		_ = conn.Close()
	}

	c.wg.Wait()
	return nil
}

// Dropped reports how many events have been discarded because the queue was
// full. It is non-zero only when handlers cannot keep up with the event rate.
func (c *Client) Dropped() uint64 {
	return c.dropped.Load()
}

// Done is closed once the client has stopped for good, whether because it was
// closed or because reconnection was abandoned.
//
// Callers need this to notice the second case. Giving up cancels a context
// derived from the caller's, which does not propagate upwards, so an app
// waiting only on its own context would sit there indefinitely holding a
// client that will never deliver another event.
func (c *Client) Done() <-chan struct{} {
	if c.ctx == nil {
		// Never connected, so it can hardly be finished. A nil channel blocks
		// forever, which is the correct answer for a select on "not yet".
		return nil
	}
	return c.ctx.Done()
}

// connectOnce performs one dial and handshake.
func (c *Client) connectOnce(ctx context.Context) (transport, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.opts.DialTimeout)
	defer cancel()

	conn, err := c.dial(dialCtx)
	if err != nil {
		return nil, err
	}

	if err := c.authenticate(dialCtx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// authenticate runs the auth handshake. It reads the connection directly, which
// is safe only because the reader loop has not started yet.
func (c *Client) authenticate(ctx context.Context, conn transport) error {
	msg, err := readOne(ctx, conn)
	if err != nil {
		return fmt.Errorf("awaiting auth_required: %w", err)
	}
	if msg.Type != typeAuthRequired {
		return fmt.Errorf("expected %s, got %s", typeAuthRequired, msg.Type)
	}

	payload, err := json.Marshal(map[string]string{
		"type":         typeAuth,
		"access_token": c.token,
	})
	if err != nil {
		return fmt.Errorf("encoding auth message: %w", err)
	}
	if err := conn.Write(ctx, payload); err != nil {
		return fmt.Errorf("sending auth message: %w", err)
	}

	msg, err = readOne(ctx, conn)
	if err != nil {
		return fmt.Errorf("awaiting auth result: %w", err)
	}

	switch msg.Type {
	case typeAuthOK:
		return nil
	case typeAuthInvalid:
		return ErrAuthFailed
	default:
		return fmt.Errorf("expected %s or %s, got %s", typeAuthOK, typeAuthInvalid, msg.Type)
	}
}

func readOne(ctx context.Context, conn transport) (Message, error) {
	raw, err := conn.Read(ctx)
	if err != nil {
		return Message{}, err
	}
	return parseMessage(raw)
}

// run owns the connection for the client's lifetime: it reads until the
// connection dies, then re-establishes it and replays the subscriptions.
func (c *Client) run(conn transport) {
	defer c.wg.Done()
	// The reader is the only sender on events, so closing here is what lets the
	// workers drain and exit.
	defer close(c.events)

	for {
		c.resubscribe()
		c.announceConnected()

		connCtx, connCancel := context.WithCancel(c.ctx)
		var keepalive sync.WaitGroup
		keepalive.Add(1)
		go func() {
			defer keepalive.Done()
			c.keepalive(connCtx)
		}()

		start := time.Now()
		err := c.readLoop(connCtx, conn)
		connCancel()
		keepalive.Wait()

		c.teardown(conn)

		if c.ctx.Err() != nil {
			return
		}
		slog.Warn("Home Assistant connection lost, reconnecting", "err", err)

		if time.Since(start) >= c.opts.HealthyAfter {
			// The connection worked for a while, so this is a fresh outage
			// rather than a continuing one.
			c.backoff.reset()
		}

		next, ok := c.reconnect()
		if !ok {
			return
		}
		conn = next
	}
}

// announceConnected runs the OnConnected hook off the reader's goroutine, so a
// slow hook costs nothing but its own time.
func (c *Client) announceConnected() {
	if c.opts.OnConnected == nil {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.opts.OnConnected()
	}()
}

// reconnect retries until it succeeds, the client is closed, or the token is
// refused. The bool reports whether a usable connection was produced.
func (c *Client) reconnect() (transport, bool) {
	for {
		delay := c.backoff.next()
		slog.Info("Reconnecting to Home Assistant", "in", delay)

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-c.ctx.Done():
			timer.Stop()
			return nil, false
		}

		conn, err := c.connectOnce(c.ctx)
		if err == nil {
			slog.Info("Reconnected to Home Assistant")
			c.setConn(conn)
			return conn, true
		}

		if errors.Is(err, ErrAuthFailed) {
			// Retrying a refused token only produces the same answer more
			// slowly, and hides the real problem behind reconnect noise.
			slog.Error("Home Assistant refused the access token, giving up", "err", err)
			c.cancel()
			return nil, false
		}
		if c.ctx.Err() != nil {
			return nil, false
		}
		slog.Warn("Reconnect attempt failed", "err", err)
	}
}

// readLoop consumes messages until the connection fails. It returns the error
// that ended it.
func (c *Client) readLoop(ctx context.Context, conn transport) error {
	reporter := dropReporter{}

	for {
		raw, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		msg, err := parseMessage(raw)
		if err != nil {
			slog.Warn("Discarding undecodable message", "err", err)
			continue
		}

		c.route(msg, &reporter)
	}
}

// route hands a message to whoever is waiting for it. It must never block: a
// reader that stalls stops handling control frames, and Home Assistant hangs up
// on a client whose messages back up for five seconds.
func (c *Client) route(msg Message, reporter *dropReporter) {
	if msg.isResult() {
		c.deliverResult(msg)
		return
	}

	if msg.Type != typeEvent {
		slog.Debug("Ignoring unsolicited message", "type", msg.Type, "id", msg.ID)
		return
	}

	// Applied here, on the reader, so ordered state is updated in wire order
	// before the workers dispatch out of order. A dropped event was still
	// applied, which is correct: the cache should reflect it even when the
	// backlog means no handler runs for it.
	if c.opts.OnEvent != nil {
		c.opts.OnEvent(msg)
	}

	select {
	case c.events <- msg:
	default:
		c.dropped.Add(1)
		reporter.record(len(c.events))
	}
}

// deliverResult completes the request this message answers.
func (c *Client) deliverResult(msg Message) {
	c.mu.Lock()
	fn, ok := c.pending[msg.ID]
	if ok {
		delete(c.pending, msg.ID)
	}
	c.mu.Unlock()

	if !ok {
		slog.Debug("Result for an unknown request", "id", msg.ID, "type", msg.Type)
		return
	}
	// Called without the lock: a waiter that re-enters the client would
	// otherwise deadlock against it.
	fn(msg)
}

// worker drains the event queue. Handlers run here, so blocking in one costs a
// worker rather than the connection.
func (c *Client) worker() {
	defer c.wg.Done()

	for msg := range c.events {
		c.mu.Lock()
		sub, ok := c.routes[msg.ID]
		c.mu.Unlock()

		if !ok {
			// Arrives for a subscription established by a previous connection,
			// or one cancelled while this message was queued.
			continue
		}
		sub.handler(msg)
	}
}

// teardown closes the connection and fails everything still waiting on it.
func (c *Client) teardown(conn transport) {
	_ = conn.Close()

	c.mu.Lock()
	c.conn = nil
	pending := c.pending
	c.pending = map[int64]func(Message){}
	c.routes = map[int64]*subscription{}
	c.mu.Unlock()

	// Ids do not survive a reconnect, so these answers are never arriving.
	for _, fn := range pending {
		fn(Message{
			Success: false,
			Error: &MessageError{
				Code:    "connection_lost",
				Message: "connection closed before Home Assistant answered",
			},
		})
	}
}

// keepalive checks liveness while the connection is otherwise idle.
//
// This sends the protocol's own ping rather than a websocket control frame: a
// control frame is answered by the HTTP server, which stays responsive even
// when Home Assistant's event loop is wedged, so it can report a connection
// healthy that will never deliver another event.
func (c *Client) keepalive(ctx context.Context) {
	ticker := time.NewTicker(c.opts.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		pingCtx, cancel := context.WithTimeout(ctx, c.opts.PingTimeout)
		_, err := c.Call(pingCtx, mapRequest{"type": typePing})
		cancel()

		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}

		slog.Warn("Ping went unanswered, dropping the connection", "err", err)
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn != nil {
			// Closing unblocks the reader, which sends run into its reconnect
			// path. Nothing else here can force that transition.
			_ = conn.Close()
		}
		return
	}
}
