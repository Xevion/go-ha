package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Xevion/go-ha/types"
)

// Send writes a request and returns as soon as it is on the wire. The result
// Home Assistant sends back is still correlated, but only to log a failure:
// without it a call_service against a missing entity fails in total silence.
func (c *Client) Send(req types.Request) error {
	_, err := c.dispatch(req, nil)
	return err
}

// Call writes a request and waits for Home Assistant to answer it.
func (c *Client) Call(ctx context.Context, req types.Request) (Message, error) {
	// Buffered so delivery never blocks the reader, even if this caller has
	// already given up and stopped waiting.
	answer := make(chan Message, 1)

	id, err := c.dispatch(req, func(msg Message) { answer <- msg })
	if err != nil {
		return Message{}, err
	}
	defer c.cancelPending(id)

	select {
	case msg := <-answer:
		return msg, msg.err()
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-c.ctx.Done():
		return Message{}, ErrClosed
	}
}

// dispatch allocates an id, registers the handler for its answer, and writes
// the request.
//
// The whole sequence runs under writeMu because Home Assistant rejects any
// message whose id is not greater than the last one it received. Allocating the
// id atomically is not enough on its own: two goroutines could take 5 and 6 and
// then reach the socket in the opposite order, at which point 5 is refused with
// "Identifier values have to increase" and that caller never gets an answer.
func (c *Client) dispatch(req types.Request, onAnswer func(Message)) (int64, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	id := c.nextID.Add(1)
	req.SetID(id)

	if onAnswer == nil {
		onAnswer = logFailure
	}

	c.mu.Lock()
	conn := c.conn
	if conn != nil {
		c.pending[id] = onAnswer
	}
	c.mu.Unlock()

	if conn == nil {
		return id, ErrNotConnected
	}

	if err := c.writeTo(conn, req); err != nil {
		c.cancelPending(id)
		return id, err
	}
	return id, nil
}

// Subscribe registers interest in an event stream. The subscription is retained
// and re-established on every subsequent connection.
func (c *Client) Subscribe(sub Subscription, handler Handler) error {
	s := &subscription{sub: sub, handler: handler}

	c.mu.Lock()
	c.subs = append(c.subs, s)
	c.mu.Unlock()

	// Establishing is a no-op while disconnected; run replays it once a
	// connection exists.
	return c.establish(s)
}

// establish sends the subscribe request for s on the current connection, and
// routes the id it allocates to it.
//
// It holds writeMu for the whole operation, which is what makes the generation
// check meaningful: without it, Subscribe and a replay could both decide a
// subscription still needed establishing and leave two streams running for the
// life of the connection.
func (c *Client) establish(s *subscription) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	conn := c.conn
	if conn == nil || s.gen == c.gen {
		c.mu.Unlock()
		return nil
	}

	id := c.nextID.Add(1)
	c.routes[id] = s
	s.gen = c.gen
	c.pending[id] = logFailure
	c.mu.Unlock()

	req := s.sub.request()
	req.SetID(id)

	if err := c.writeTo(conn, req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		delete(c.routes, id)
		// Leave gen behind so the next replay retries this subscription.
		s.gen = 0
		c.mu.Unlock()
		return err
	}
	return nil
}

// resubscribe replays every subscription not yet established on the current
// connection. Ids are connection-scoped, so each replay allocates a fresh one.
func (c *Client) resubscribe() {
	c.mu.Lock()
	subs := append([]*subscription(nil), c.subs...)
	c.mu.Unlock()

	for _, s := range subs {
		if err := c.establish(s); err != nil {
			slog.Error("Failed to replay a subscription", "err", err)
		}
	}
}

// logFailure is the answer handler for requests whose outcome is only worth
// reporting, rather than waiting on.
func logFailure(msg Message) {
	if err := msg.err(); err != nil {
		slog.Error("Home Assistant rejected a request", "id", msg.ID, "err", err)
	}
}

func (c *Client) cancelPending(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

// writeTo encodes and sends a message on the given connection. Callers hold
// writeMu, which keeps ids reaching the socket in the order they were taken.
func (c *Client) writeTo(conn transport, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	if c.ctx == nil {
		return ErrNotConnected
	}
	ctx, cancel := context.WithTimeout(c.ctx, c.opts.WriteTimeout)
	defer cancel()

	if err := conn.Write(ctx, data); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}
