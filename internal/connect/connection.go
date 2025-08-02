package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/gorilla/websocket"
)

var ErrInvalidToken = errors.New("invalid authentication token")

// HAConnection is a wrapper around a WebSocket connection that provides a mutex for thread safety.
type HAConnection struct {
	Conn  *websocket.Conn // Note: this is not thread safe except for Close() and WriteControl()
	mutex sync.Mutex
}

// WriteMessage writes a message to the WebSocket connection.
func (w *HAConnection) WriteMessage(msg any) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	return w.Conn.WriteJSON(msg)
}

// ReadMessageRaw reads a raw message from the WebSocket connection.
func ReadMessageRaw(conn *websocket.Conn) ([]byte, error) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// ReadMessage reads a message from the WebSocket connection and unmarshals it into the given type.
func ReadMessage[T any](conn *websocket.Conn) (T, error) {
	var result T
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(msg, &result)
	if err != nil {
		return result, err
	}

	return result, nil
}

// ConnectionFromUri creates a new WebSocket connection from the given base URL and authentication token.
func ConnectionFromUri(baseUrl *url.URL, token string) (*HAConnection, context.Context, context.CancelFunc, error) {
	// Build the WebSocket URL
	urlWebsockets := *baseUrl
	urlWebsockets.Path = "/api/websocket"
	scheme, err := internal.GetEquivalentWebsocketScheme(baseUrl.Scheme)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to build WebSocket URL: %w", err)
	}
	urlWebsockets.Scheme = scheme

	// Create a short timeout context for the connection only
	connCtx, connCtxCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer connCtxCancel() // Always cancel the connection context when we're done

	// Init WebSocket connection
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(connCtx, urlWebsockets.String(), nil)
	if err != nil {
		slog.Error("Failed to connect to WebSocket. Check URI\n", "url", urlWebsockets)
		return nil, nil, nil, err
	}

	// Read auth_required message
	msg, err := ReadMessage[struct {
		MsgType string `json:"type"`
	}](conn)
	if err != nil {
		slog.Error("Unknown error creating WebSocket client\n")
		return nil, nil, nil, err
	} else if msg.MsgType != "auth_required" {
		slog.Error("Expected auth_required message, got", "msgType", msg.MsgType)
		return nil, nil, nil, fmt.Errorf("expected auth_required message, got %s", msg.MsgType)
	}

	// Send auth message
	err = SendAuthMessage(conn, connCtx, token)
	if err != nil {
		slog.Error("Unknown error creating WebSocket client\n")
		return nil, nil, nil, err
	}

	// Verify auth message was successful
	err = VerifyAuthResponse(conn, connCtx)
	if err != nil {
		slog.Error("Auth token is invalid. Please double check it or create a new token in your Home Assistant profile\n")
		return nil, nil, nil, err
	}

	// Create a new background context for the application lifecycle (no timeout)
	appCtx, appCtxCancel := context.WithCancel(context.Background())

	return &HAConnection{Conn: conn}, appCtx, appCtxCancel, nil
}

// SendAuthMessage sends an auth message to the WebSocket connection.
func SendAuthMessage(conn *websocket.Conn, ctx context.Context, token string) error {
	type AuthMessage struct {
		MsgType     string `json:"type"`
		AccessToken string `json:"access_token"`
	}

	err := conn.WriteJSON(AuthMessage{MsgType: "auth", AccessToken: token})
	if err != nil {
		return err
	}

	return nil
}

// VerifyAuthResponse verifies that the auth response is valid.
func VerifyAuthResponse(conn *websocket.Conn, ctx context.Context) error {
	msg, err := ReadMessage[struct {
		MsgType string `json:"type"`
		Message string `json:"message"`
	}](conn)
	if err != nil {
		return err
	}

	if msg.MsgType != "auth_ok" {
		return ErrInvalidToken
	}

	return nil
}

func SubscribeToStateChangedEvents(id int64, conn *HAConnection, ctx context.Context) {
	SubscribeToEventType("state_changed", conn, ctx, id)
}

// TODO: Instead of using variadic arguments, just use a nillable pointer for the id
func SubscribeToEventType(eventType string, conn *HAConnection, ctx context.Context, id ...int64) {
	type SubEvent struct {
		Id        int64  `json:"id"`
		Type      string `json:"type"`
		EventType string `json:"event_type"`
	}

	// If no id is provided, generate a new one
	var finalId int64
	if len(id) == 0 {
		finalId = internal.NextId()
	} else {
		finalId = id[0]
	}
	e := SubEvent{
		Id:        finalId,
		Type:      "subscribe_events",
		EventType: eventType,
	}

	err := conn.WriteMessage(e)
	// TODO: Handle errors better
	if err != nil {
		wrappedErr := fmt.Errorf("error writing to WebSocket: %w", err)
		slog.Error(wrappedErr.Error())
		panic(wrappedErr)
	}
}
