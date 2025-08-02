package internal

import (
	"fmt"
	"reflect"
	"runtime"
	"sync/atomic"
)

type EnabledDisabledInfo struct {
	Entity     string
	State      string
	RunOnError bool
}

var (
	currentVersion = "0.7.1"
)

var (
	id atomic.Int64 // default value is 0
)

// NextId returns a unique integer (for the given process), often used for providing a uniquely identifiable ID for a request. This function is thread-safe.
func NextId() int64 {
	return id.Add(1)
}

// GetFunctionName returns the name of the function that the interface is a pointer to.
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// GetEquivalentWebsocketScheme returns the equivalent WebSocket scheme for the given scheme.
// If the scheme is http or https, it returns ws or wss respectively.
// If the scheme is ws or wss, it returns the same scheme.
// If the scheme is not any of the above, it returns an error.
func GetEquivalentWebsocketScheme(scheme string) (string, error) {
	switch scheme {
	case "http":
		return "ws", nil
	case "https":
		return "wss", nil
	case "ws", "wss":
		return scheme, nil
	default:
		return "", fmt.Errorf("unexpected scheme: %s", scheme)
	}
}
