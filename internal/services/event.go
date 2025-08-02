package services

import (
	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
)

type Event struct {
	conn *connect.HAConnection
}

// Fire an event
type FireEventRequest struct {
	Id        int64          `json:"id"`
	Type      string         `json:"type"` // always set to "fire_event"
	EventType string         `json:"event_type"`
	EventData map[string]any `json:"event_data,omitempty"`
}

// Fire an event. Takes an event type and an optional map that is sent as `event_data`.
func (e Event) Fire(eventType string, eventData ...map[string]any) error {
	req := FireEventRequest{
		Id:   internal.NextId(),
		Type: "fire_event",
	}

	req.EventType = eventType
	if len(eventData) != 0 {
		req.EventData = eventData[0]
	}

	return e.conn.WriteMessage(req)
}
