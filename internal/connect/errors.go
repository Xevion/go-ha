package connect

import "errors"

var (
	// ErrAuthFailed reports a token Home Assistant refused. It is terminal:
	// retrying a rejected token only produces the same answer more slowly.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrNotConnected reports a send attempted while no connection was live.
	ErrNotConnected = errors.New("not connected")

	// ErrUnexpectedFrame reports a non-text websocket frame, which the Home
	// Assistant protocol never sends.
	ErrUnexpectedFrame = errors.New("unexpected websocket frame type")

	// ErrClosed reports use of a client whose context has already been cancelled.
	ErrClosed = errors.New("client closed")

	// ErrCallFailed reports a request Home Assistant answered with success=false.
	ErrCallFailed = errors.New("call failed")
)
