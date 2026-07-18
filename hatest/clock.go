package hatest

import (
	"sync"
	"time"
)

// Clock is a time source a test drives by hand. Give one to
// types.NewAppRequest to make schedules, throttles and For durations resolve
// on demand instead of on the wall clock.
type Clock struct {
	mu  sync.RWMutex
	now time.Time
}

// NewClock returns a clock parked at the given instant.
func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

// Now reports the current instant. It is read from automation callbacks, which
// run on their own goroutines, so it is guarded.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

// Set replaces the current instant.
func (c *Clock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

// Advance moves the clock forward. A negative duration moves it back.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
