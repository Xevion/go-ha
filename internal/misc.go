package internal

import (
	"fmt"
	"reflect"
	"runtime"
)

type EnabledDisabledInfo struct {
	Entity     string
	State      string
	RunOnError bool
}

// Version is the go-ha release this build corresponds to.
const Version = "0.8.0"

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

func Ptr[T any](v T) *T {
	return &v
}
