package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/internal"
)

// noRun is a run that returns immediately.
func noRun(context.Context) {}

// blocking returns a run that parks until the returned channel is closed.
func blocking() (func(context.Context), chan struct{}, *atomic.Int64) {
	release := make(chan struct{})
	var entered atomic.Int64

	return func(ctx context.Context) {
		entered.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}, release, &entered
}

func TestThrottleDropsTriggersInsideTheWindow(t *testing.T) {
	clock := testClock()
	r := newRunner(Policy{Mode: ModeParallel, Throttle: 5 * time.Minute}, clock)

	assert.True(t, r.run(context.Background(), "light.kitchen", noRun))
	assert.False(t, r.run(context.Background(), "light.kitchen", noRun))

	clock.Advance(5 * time.Minute)
	assert.True(t, r.run(context.Background(), "light.kitchen", noRun),
		"the window has passed")

	r.wait()
}

// One automation can watch many entities. A shared throttle window lets a busy
// entity starve every other one, which is the bug a single lastRan field had.
func TestThrottleIsKeyedPerEntity(t *testing.T) {
	clock := testClock()
	r := newRunner(Policy{Mode: ModeParallel, Throttle: 5 * time.Minute}, clock)

	require.True(t, r.run(context.Background(), "sensor.busy", noRun))
	require.False(t, r.run(context.Background(), "sensor.busy", noRun))

	assert.True(t, r.run(context.Background(), "sensor.quiet", noRun),
		"a different entity has its own throttle window")

	r.wait()
}

func TestSingleIgnoresTriggersWhileRunning(t *testing.T) {
	r := newRunner(Policy{Mode: ModeSingle}, testClock())
	fn, release, entered := blocking()

	require.True(t, r.run(context.Background(), "key", fn))
	waitFor(t, entered, 1)

	assert.False(t, r.run(context.Background(), "key", fn), "a run is already in flight")

	close(release)
	r.wait()

	assert.True(t, r.run(context.Background(), "key", noRun), "admitted again once idle")
	r.wait()
}

// Mode is automation-wide, as it is in Home Assistant: "single" there means one
// run of the automation, not one per entity. Throttle is the per-entity half.
func TestSingleAppliesAcrossEntities(t *testing.T) {
	r := newRunner(Policy{Mode: ModeSingle}, testClock())
	fn, release, entered := blocking()

	require.True(t, r.run(context.Background(), "light.slow", fn))
	waitFor(t, entered, 1)

	assert.False(t, r.run(context.Background(), "light.other", fn),
		"single means one run of the automation, whichever entity triggered it")

	close(release)
	r.wait()
}

func TestRunnerUsesTheInjectedClock(t *testing.T) {
	clock := testClock()
	r := newRunner(Policy{Mode: ModeParallel, Throttle: time.Hour}, internal.RealClock{})
	r.withClock(clock)

	require.True(t, r.run(context.Background(), "key", noRun))
	require.False(t, r.run(context.Background(), "key", noRun))

	clock.Advance(time.Hour)
	assert.True(t, r.run(context.Background(), "key", noRun),
		"the throttle window must move with the clock conditions read")

	r.wait()
}

func TestRestartCancelsTheRunInFlight(t *testing.T) {
	r := newRunner(Policy{Mode: ModeRestart}, testClock())

	cancelled := make(chan struct{})
	var once sync.Once
	first := func(ctx context.Context) {
		<-ctx.Done()
		once.Do(func() { close(cancelled) })
	}

	require.True(t, r.run(context.Background(), "key", first))
	require.True(t, r.run(context.Background(), "key", noRun))

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("restart did not cancel the run in flight")
	}

	r.wait()
}

func TestQueuedRunsDoNotOverlap(t *testing.T) {
	r := newRunner(Policy{Mode: ModeQueued}, testClock())

	var concurrent, peak atomic.Int64
	var done sync.WaitGroup

	fn := func(context.Context) {
		defer done.Done()
		n := concurrent.Add(1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		concurrent.Add(-1)
	}

	for range 4 {
		done.Add(1)
		require.True(t, r.run(context.Background(), "key", fn))
	}

	done.Wait()
	r.wait()
	assert.Equal(t, int64(1), peak.Load(), "queued runs must never overlap")
}

func TestQueuedDropsPastItsLimit(t *testing.T) {
	r := newRunner(Policy{Mode: ModeQueued, Limit: 2}, testClock())
	fn, release, entered := blocking()

	// The first is admitted and runs; the next two wait; the fourth has
	// nowhere to go.
	require.True(t, r.run(context.Background(), "key", fn))
	waitFor(t, entered, 1)
	require.True(t, r.run(context.Background(), "key", fn))
	require.True(t, r.run(context.Background(), "key", fn))
	assert.False(t, r.run(context.Background(), "key", fn), "the queue is full")

	close(release)
	r.wait()
}

func TestParallelRunsConcurrently(t *testing.T) {
	r := newRunner(Policy{Mode: ModeParallel}, testClock())
	fn, release, entered := blocking()

	require.True(t, r.run(context.Background(), "key", fn))
	require.True(t, r.run(context.Background(), "key", fn))
	waitFor(t, entered, 2)

	close(release)
	r.wait()
}

func TestParallelStopsAtItsLimit(t *testing.T) {
	r := newRunner(Policy{Mode: ModeParallel, Limit: 2}, testClock())
	fn, release, entered := blocking()

	require.True(t, r.run(context.Background(), "key", fn))
	require.True(t, r.run(context.Background(), "key", fn))
	waitFor(t, entered, 2)

	assert.False(t, r.run(context.Background(), "key", fn), "the limit is reached")

	close(release)
	r.wait()
}

// waitFor blocks until the counter reaches n, so tests synchronise on progress
// rather than on a sleep long enough to probably be sufficient.
func waitFor(t *testing.T, counter *atomic.Int64, n int64) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	for counter.Load() < n {
		select {
		case <-deadline:
			t.Fatalf("expected %d runs to start, got %d", n, counter.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
