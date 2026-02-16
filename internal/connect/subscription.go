package connect

// Subscription declares an interest in a stream of Home Assistant events.
//
// It is data rather than an action so the client can replay it. Subscribing
// imperatively at registration time means a reconnect silently loses every
// listener: Home Assistant replays nothing, and the ids handed out by the old
// connection are meaningless on the new one.
type Subscription struct {
	// EventType names the event to receive. An empty value subscribes to every
	// event Home Assistant emits.
	EventType string
}

// Handler receives each message delivered for a subscription. It runs on a
// worker goroutine, so it may block without stalling the reader.
type Handler func(Message)

// subscription pairs a declaration with the handler that consumes it.
type subscription struct {
	sub     Subscription
	handler Handler
	// gen is the connection generation this was last established for, and is
	// what stops a replay from duplicating a subscription that Subscribe has
	// already sent on the new connection.
	gen uint64
}

// request builds the wire message that establishes this subscription. The id is
// stamped by the client at send time, since it is only valid for one connection.
func (s Subscription) request() mapRequest {
	req := mapRequest{"type": typeSubscribeEvents}
	if s.EventType != "" {
		req["event_type"] = s.EventType
	}
	return req
}

// Request is a message the client stamps with a connection-scoped id before
// sending. Ids are allocated per client rather than per process, so two Apps in
// one binary no longer share a counter.
type Request interface {
	SetID(id int64)
}

// mapRequest is an ad-hoc request built inline, for protocol messages that have
// no dedicated type.
type mapRequest map[string]any

func (m mapRequest) SetID(id int64) { m["id"] = id }
