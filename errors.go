package ha

import (
	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
)

// Errors this package returns, re-exported so callers can classify a failure
// with errors.Is rather than matching on message text.
var (
	// ErrEntityNotFound reports an entity Home Assistant does not know about.
	ErrEntityNotFound = internal.ErrEntityNotFound

	// ErrUnauthorized reports a token Home Assistant refused.
	ErrUnauthorized = internal.ErrUnauthorized

	// ErrHTTPStatus reports any other unsuccessful REST response.
	ErrHTTPStatus = internal.ErrHttpStatus

	// ErrNotConnected reports a call made while the websocket was down.
	ErrNotConnected = connect.ErrNotConnected

	// ErrAuthFailed reports a rejected websocket handshake.
	ErrAuthFailed = connect.ErrAuthFailed
)
