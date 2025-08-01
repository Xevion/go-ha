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

func GetId() int64 {
	return id.Add(1)
}

// GetFunctionName returns the name of the function that the interface is a pointer to.
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
