package internal

import (
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
	id atomic.Int64 // default value is 0
)

// NextId returns a unique integer (for the given process), often used for providing a uniquely identifiable
// id for a request. This function is thread-safe.
func NextId() int64 {
	return id.Add(1)
}

// GetFunctionName returns the name of the function that the interface is a pointer to.
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
