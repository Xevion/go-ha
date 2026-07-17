package types

// NewAppRequest contains the configuration for creating a new App instance.
type NewAppRequest struct {
	// Required
	// URL of your Home Assistant instance, e.g. "http://localhost:8123".
	// The scheme decides whether the connection is plain or TLS.
	URL string

	// Required
	// Auth token generated in Home Assistant. Used
	// to connect to the WebSocket API.
	HAAuthToken string

	// Optional
	// Connection tunes the websocket connection. The zero value uses defaults
	// suitable for a typical Home Assistant instance.
	Connection ConnectionOptions
}
