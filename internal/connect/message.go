package connect

import (
	"encoding/json"
	"fmt"
)

// Home Assistant websocket message types, as sent over the wire.
const (
	typeAuthRequired    = "auth_required"
	typeAuth            = "auth"
	typeAuthOK          = "auth_ok"
	typeAuthInvalid     = "auth_invalid"
	typeResult          = "result"
	typeEvent           = "event"
	typePing            = "ping"
	typePong            = "pong"
	typeSubscribeEvents = "subscribe_events"
)

// Message is a decoded frame from Home Assistant. Raw is retained because
// callers decode their own event payloads out of it.
type Message struct {
	ID      int64
	Type    string
	Success bool
	Raw     []byte
	Error   *MessageError
}

// MessageError is the error object Home Assistant attaches to a failed result.
type MessageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *MessageError) Error() string {
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

// envelope mirrors the fields common to every message Home Assistant sends.
type envelope struct {
	ID      int64         `json:"id"`
	Type    string        `json:"type"`
	Success *bool         `json:"success"`
	Error   *MessageError `json:"error"`
}

// parseMessage decodes the envelope shared by all messages, leaving the payload
// in Raw for whoever handles this message type.
func parseMessage(raw []byte) (Message, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Message{}, fmt.Errorf("decoding message envelope: %w", err)
	}

	// Only results carry "success"; treat its absence as success so events and
	// pongs are not mistaken for failures.
	success := true
	if env.Success != nil {
		success = *env.Success
	}

	return Message{
		ID:      env.ID,
		Type:    env.Type,
		Success: success,
		Raw:     raw,
		Error:   env.Error,
	}, nil
}

// isResult reports whether this message answers a request the client sent,
// rather than carrying a subscribed event.
func (m Message) isResult() bool {
	return m.Type == typeResult || m.Type == typePong
}

// err returns the failure this message reports, or nil when it succeeded.
func (m Message) err() error {
	if m.Success {
		return nil
	}
	if m.Error != nil {
		return fmt.Errorf("%w: %s", ErrCallFailed, m.Error.Error())
	}
	return ErrCallFailed
}
