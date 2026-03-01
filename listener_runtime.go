package ha

import (
	"sync"
	"time"

	"github.com/dromara/carbon/v2"

	"github.com/Xevion/go-ha/internal"
)

// listenerRuntime holds the state a listener accumulates while it runs.
//
// It sits behind a pointer because listeners are copied by value the whole way
// through their builder chains, and a mutex may not be copied. Events are
// dispatched from a pool of workers, so every field here is reachable from
// several goroutines at once.
type listenerRuntime struct {
	mu         sync.Mutex
	lastRan    *carbon.Carbon
	delayTimer *time.Timer
}

func newListenerRuntime() *listenerRuntime {
	return &listenerRuntime{lastRan: carbon.Now().StartOfCentury()}
}

// throttled reports whether the throttle window is still open, without
// claiming the slot. It is a cheap reject before the condition checks that
// reach Home Assistant over HTTP.
func (r *listenerRuntime) throttled(clock internal.Clock, throttle time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CheckThrottle(clock, throttle, r.lastRan).fail
}

// claim reports whether this listener may fire now, stamping the run time if
// so.
//
// Checking and stamping are deliberately one critical section. Split apart,
// two workers handling events for the same entity both read the same lastRan,
// both decide they are outside the throttle window, and both fire, which is
// exactly what a throttle exists to prevent.
func (r *listenerRuntime) claim(clock internal.Clock, throttle time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if CheckThrottle(clock, throttle, r.lastRan).fail {
		return false
	}
	r.lastRan = clock.Carbon()
	return true
}

// stamp records a run that happened outside claim, on the delayed path.
func (r *listenerRuntime) stamp(clock internal.Clock) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRan = clock.Carbon()
}

// arm replaces the pending delay timer, cancelling any already waiting.
func (r *listenerRuntime) arm(timer *time.Timer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.delayTimer != nil {
		r.delayTimer.Stop()
	}
	r.delayTimer = timer
}

// disarm cancels a pending delay timer, if there is one.
func (r *listenerRuntime) disarm() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.delayTimer != nil {
		r.delayTimer.Stop()
	}
}
