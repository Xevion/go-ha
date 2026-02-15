package connect

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coder/websocket"

	"github.com/Xevion/go-ha/internal"
)

// Home Assistant sends the full attribute payload with every state_changed
// event, and a single camera or media_player entity can carry a large one.
// coder/websocket caps reads at 32 KiB by default, which gorilla did not do at
// all, so the limit has to be raised explicitly or the migration would start
// dropping connections on entities that used to work.
const readLimit = 16 << 20

// transport is the subset of a websocket connection the client depends on.
// Keeping it this narrow lets the reconnect and dispatch logic run against an
// in-memory pipe, where a real socket would stop synctest's clock and make
// every backoff test a real-time sleep.
type transport interface {
	// Read returns the next message. The client never calls it concurrently.
	Read(ctx context.Context) ([]byte, error)
	// Write sends a single message and is safe to call alongside Read.
	Write(ctx context.Context, data []byte) error
	// Close releases the connection and unblocks any pending Read.
	Close() error
}

// dialer opens a fresh transport, and is called once per connection attempt.
type dialer func(ctx context.Context) (transport, error)

// wsTransport adapts a coder/websocket connection to the transport interface.
// It carries no mutex: unlike gorilla, coder/websocket serialises concurrent
// writes internally.
type wsTransport struct {
	conn *websocket.Conn
}

func (w *wsTransport) Read(ctx context.Context) ([]byte, error) {
	typ, data, err := w.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	if typ != websocket.MessageText {
		return nil, fmt.Errorf("%w: %v", ErrUnexpectedFrame, typ)
	}
	return data, nil
}

func (w *wsTransport) Write(ctx context.Context, data []byte) error {
	return w.conn.Write(ctx, websocket.MessageText, data)
}

func (w *wsTransport) Close() error {
	return w.conn.Close(websocket.StatusNormalClosure, "")
}

// websocketDialer returns a dialer that opens a real connection to the Home
// Assistant websocket endpoint derived from baseUrl.
func websocketDialer(baseUrl *url.URL) (dialer, error) {
	endpoint := *baseUrl
	endpoint.Path = "/api/websocket"
	scheme, err := internal.GetEquivalentWebsocketScheme(baseUrl.Scheme)
	if err != nil {
		return nil, fmt.Errorf("building websocket url: %w", err)
	}
	endpoint.Scheme = scheme
	target := endpoint.String()

	return func(ctx context.Context) (transport, error) {
		conn, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{})
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("dialing %s: %w", target, ErrAuthFailed)
			}
			return nil, fmt.Errorf("dialing %s: %w", target, err)
		}
		conn.SetReadLimit(readLimit)
		return &wsTransport{conn: conn}, nil
	}, nil
}
