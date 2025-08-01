package types

// NewAppRequest contains the configuration for creating a new App instance.
type NewAppRequest struct {
	// Required
	URL string

	// Optional
	// Deprecated: use URL instead
	// IpAddress of your Home Assistant instance i.e. "localhost"
	// or "192.168.86.59" etc.
	IpAddress string

	// Optional
	// Deprecated: use URL instead
	// Port number Home Assistant is running on. Defaults to 8123.
	Port string

	// Required
	// Auth token generated in Home Assistant. Used
	// to connect to the Websocket API.
	HAAuthToken string

	// Required
	// EntityId of the zone representing your home e.g. "zone.home".
	// Used to pull latitude/longitude from Home Assistant
	// to calculate sunset/sunrise times.
	HomeZoneEntityId string

	// Optional
	// Whether to use secure connections for http and websockets.
	// Setting this to `true` will use `https://` instead of `https://`
	// and `wss://` instead of `ws://`.
	Secure bool
}
