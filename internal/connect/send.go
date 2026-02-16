package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Send writes a request and returns as soon as it is on the wire. The result
// Home Assistant sends back is still correlated, but only to log a failure:
// without it a call_service against a missing entity fails in total silence.
func (c *Client) Send(req Request) error {
	id := c.nextID.Add(1)
	req.SetID(id)

	c.watchResult(id)
	if err := c.write(req); err != nil {
		c.cancelPending(id)
		return err
	}
	return nil
}

// Call writes a request and waits for Home Assistant to answer it.
func (c *Client) Call(ctx context.Context, req Request) (Message, error) {
	id := c.nextID.Add(1)
	req.SetID(id)

	// Buffered so delivery never blocks the reader, even if this caller has
	// already given up and stopped waiting.
	answer := make(chan Message, 1)
	c.setPending(id, func(msg Message) { answer <- msg })
	defer c.cancelPending(id)

	if err := c.write(req); err != nil {
		return Message{}, err
	}

	select {
	case msg := <-answer:
		return msg, msg.err()
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-c.ctx.Done():
		return Message{}, ErrClosed
	}
}

// Subscribe registers interest in an event stream. The subscription is retained
// and re-established on every subsequent connection.
func (c *Client) Subscribe(sub Subscription, handler Handler) error {
	s := &subscription{sub: sub, handler: handler}

	c.mu.Lock()
	c.subs = append(c.subs, s)
	if c.conn == nil {
		// Nothing to send yet; run replays it once a connection exists.
		c.mu.Unlock()
		return nil
	}
	req := c.establishLocked(s)
	c.mu.Unlock()

	return c.sendEstablished(req)
}

// establishLocked allocates an id for a subscription on the current connection
// and routes that id to it. The caller must hold c.mu.
func (c *Client) establishLocked(s *subscription) mapRequest {
	id := c.nextID.Add(1)
	c.routes[id] = s
	s.gen = c.gen

	req := s.sub.request()
	req.SetID(id)
	return req
}

// resubscribe replays every subscription not yet established on the current
// connection. Ids are connection-scoped, so each replay allocates a fresh one.
func (c *Client) resubscribe() {
	c.mu.Lock()
	if c.conn == nil {
		c.mu.Unlock()
		return
	}

	var reqs []mapRequest
	for _, s := range c.subs {
		// A subscription added by Subscribe between the reconnect and now has
		// already been established for this generation; sending again would
		// leave a duplicate stream running for the life of the connection.
		if s.gen == c.gen {
			continue
		}
		reqs = append(reqs, c.establishLocked(s))
	}
	c.mu.Unlock()

	if len(reqs) == 0 {
		return
	}
	slog.Info("Replaying subscriptions", "count", len(reqs))

	for _, req := range reqs {
		if err := c.sendEstablished(req); err != nil {
			slog.Error("Failed to replay a subscription", "err", err)
		}
	}
}

// sendEstablished writes a request whose id was assigned in advance.
func (c *Client) sendEstablished(req mapRequest) error {
	id, ok := req["id"].(int64)
	if !ok {
		return fmt.Errorf("request has no id assigned")
	}

	c.watchResult(id)
	if err := c.write(req); err != nil {
		c.cancelPending(id)
		return err
	}
	return nil
}

// watchResult records a request whose outcome is only worth logging.
func (c *Client) watchResult(id int64) {
	c.setPending(id, func(msg Message) {
		if err := msg.err(); err != nil {
			slog.Error("Home Assistant rejected a request", "id", msg.ID, "err", err)
		}
	})
}

func (c *Client) setPending(id int64, fn func(Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[id] = fn
}

func (c *Client) cancelPending(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

// write encodes and sends a message on the current connection.
func (c *Client) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return ErrNotConnected
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
