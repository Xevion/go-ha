package ha

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Xevion/go-ha/internal"
)

func testClock() *internal.FakeClock {
	return internal.NewFakeClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
}

func TestListenerRuntimeClaimAdmitsOneRunPerWindow(t *testing.T) {
	clock := testClock()
	r := newListenerRuntime()

	var admitted atomic.Int64
	var wg sync.WaitGroup

	// Released together, so they contend for the throttle at once rather than
	// drifting into a sequence the scheduler happens to serialise.
	start := make(chan struct{})
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if r.claim(clock, time.Minute) {
				admitted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	// Events for one entity are dispatched across a pool of workers. Checking
	// the throttle and stamping it separately let every one of these read the
	// same lastRan and decide it was clear to run.
	assert.Equal(t, int64(1), admitted.Load(),
		"a throttle must admit exactly one run per window, however many workers race for it")
}

func TestListenerRuntimeClaimAdmitsAgainAfterTheWindow(t *testing.T) {
	clock := testClock()
	r := newListenerRuntime()

	assert.True(t, r.claim(clock, time.Minute))
	assert.False(t, r.claim(clock, time.Minute))

	clock.Advance(2 * time.Minute)
	assert.True(t, r.claim(clock, time.Minute), "the window must reopen once it has elapsed")
}

func TestListenerRuntimeWithoutThrottleAlwaysAdmits(t *testing.T) {
	clock := testClock()
	r := newListenerRuntime()

	for range 5 {
		assert.True(t, r.claim(clock, 0), "an unthrottled listener must never be held back")
	}
}

func TestListenerRuntimeThrottledDoesNotClaim(t *testing.T) {
	clock := testClock()
	r := newListenerRuntime()

	// The early reject must not consume the window, or the claim that follows
	// it would always fail and the listener would never fire.
	assert.False(t, r.throttled(clock, time.Minute))
	assert.False(t, r.throttled(clock, time.Minute))
	assert.True(t, r.claim(clock, time.Minute))
}

func TestListenerRuntimeTimerHandoffIsSafe(t *testing.T) {
	r := newListenerRuntime()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.arm(time.AfterFunc(time.Hour, func() {}))
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.disarm()
		}()
	}
	wg.Wait()

	r.disarm()
}

func TestListenerRuntimeStampBlocksTheNextClaim(t *testing.T) {
	clock := testClock()
	r := newListenerRuntime()

	// The delayed path stamps when the timer fires rather than when the event
	// arrived, and that stamp has to count against the throttle.
	r.stamp(clock)
	assert.False(t, r.claim(clock, time.Minute))
}
