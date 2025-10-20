package internal

import (
	"sync"
	"time"

	"github.com/dromara/carbon/v2"
)

// Clock reports the current time. Production code uses RealClock; tests pin an
// instant with FakeClock and step it forward deliberately.
type Clock interface {
	Now() time.Time
	Carbon() *carbon.Carbon
}

// RealClock reads the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

func (RealClock) Carbon() *carbon.Carbon {
	return carbon.Now()
}

// FakeClock reports a fixed instant until moved by Set or Advance. Callbacks run
// on their own goroutines and read the clock freely, so access is guarded.
type FakeClock struct {
	mutex sync.RWMutex
	now   time.Time
}

// NewFakeClock returns a FakeClock pinned to the given instant.
func NewFakeClock(now time.Time) *FakeClock {
	return &FakeClock{now: now}
}

func (c *FakeClock) Now() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.now
}

func (c *FakeClock) Carbon() *carbon.Carbon {
	return carbon.CreateFromStdTime(c.Now())
}

// Set replaces the current instant.
func (c *FakeClock) Set(now time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.now = now
}

// Advance moves the clock forward by d. Negative durations move it back.
func (c *FakeClock) Advance(d time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.now = c.now.Add(d)
}
