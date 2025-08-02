package connect

import (
	"encoding/json"
	"log/slog"

	"github.com/gorilla/websocket"
)

// BaseMessage is the base message type for all messages sent by the websocket server.
type BaseMessage struct {
	Type    string `json:"type"`
	Id      int64  `json:"id"`
	Success bool   `json:"success"` // not present in all messages
}

type ChannelMessage struct {
	Id      int64
	Type    string
	Success bool
	Raw     []byte
}

// ListenWebsocket reads messages from the websocket connection and sends them to the channel.
// It will close the channel if it encounters an error, or if the channel is full, and return.
// It ignores errors in deserialization.
func ListenWebsocket(conn *websocket.Conn, c chan ChannelMessage) {
	for {
		raw, err := ReadMessageRaw(conn)
		if err != nil {
			slog.Error("Error reading from websocket", "err", err)
			close(c)
			break
		}

		base := BaseMessage{
			// default to true for messages that don't include "success" at all
			Success: true,
		}
		err = json.Unmarshal(raw, &base)
		if err != nil {
			slog.Error("Error unmarshalling message", "err", err, "message", string(raw))
			continue
		}
		if !base.Success {
			slog.Warn("Received unsuccessful response", "response", string(raw))
		}

		// Create a channel message from the raw message
		channelMessage := ChannelMessage{
			Type:    base.Type,
			Id:      base.Id,
			Success: base.Success,
			Raw:     raw,
		}

		// Use non-blocking send to avoid hanging on closed channel
		select {
		case c <- channelMessage:
			// Message sent successfully
		default:
			// Channel is full or closed, break out of loop
			slog.Warn("Websocket message channel is full or closed, stopping listener",
				"channel_capacity", cap(c),
				"channel_length", len(c))
			close(c)
			return
		}
	}
}
