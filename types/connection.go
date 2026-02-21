package types

import "time"

// ConnectionOptions tunes the websocket connection. The zero value selects a
// sensible default for every field, so only the settings you care about need
// to be set.
//
// This mirrors the internal connection options rather than exposing them
// directly, which keeps the wire layer free to change shape without moving the
// public API with it.
type ConnectionOptions struct {
	// QueueSize bounds how many events may be waiting for a handler.
	//
	// Home Assistant disconnects a client that stops draining its socket, so
	// the queue is deliberately finite: once it is full, further events are
	// dropped and counted rather than allowed to stall the connection. Raise it
	// if your handlers are bursty, and prefer making handlers return quickly
	// over setting it very high. Defaults to 256.
	QueueSize int

	// Workers is how many events may be handled concurrently. A handler that
	// blocks occupies a worker for as long as it runs, so this is effectively
	// the number of slow callbacks tolerated before events start queueing.
	// Defaults to 4.
	Workers int

	// PingInterval is how often an idle connection is checked for liveness.
	// Defaults to 30 seconds.
	PingInterval time.Duration
}
