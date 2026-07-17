package ha

import (
	"context"
	"sync"
	"time"

	"github.com/Xevion/go-ha/internal"
)

// Mode decides what a trigger does while a previous run of the same automation
// is still going. The names mirror Home Assistant's automation mode, so a
// config translated from YAML behaves the same way here.
type Mode int

const (
	// ModeSingle ignores triggers while a run is in flight. It is the default,
	// as it is in Home Assistant.
	ModeSingle Mode = iota

	// ModeRestart cancels the run in flight and starts again. The cancelled
	// run sees its context done.
	ModeRestart

	// ModeQueued admits the trigger but holds it until the run in flight
	// finishes, so runs never overlap.
	ModeQueued

	// ModeParallel runs triggers concurrently.
	ModeParallel
)

func (m Mode) String() string {
	switch m {
	case ModeRestart:
		return "restart"
	case ModeQueued:
		return "queued"
	case ModeParallel:
		return "parallel"
	default:
		return "single"
	}
}

// Policy governs how often and how concurrently an automation runs.
type Policy struct {
	// Mode decides what happens when a run is already in flight.
	Mode Mode

	// Throttle drops triggers arriving within this long of the last admitted
	// one.
	Throttle time.Duration

	// Limit caps in-flight runs under ModeParallel and waiting runs under
	// ModeQueued. Zero means the default.
	Limit int
}

const defaultLimit = 10

func (p Policy) limit() int {
	if p.Limit > 0 {
		return p.Limit
	}
	return defaultLimit
}

// runner enforces one automation's policy.
//
// The two halves are scoped differently on purpose. Mode is automation-wide,
// as it is in Home Assistant: "single" there means one run of the automation,
// not one per entity. Throttle is per entity, because an automation watching
// many of them otherwise lets a busy entity consume the window belonging to
// every other one.
type runner struct {
	policy Policy
	clock  internal.Clock

	mu sync.Mutex

	// lastRan holds the last admitted run per throttle key.
	lastRan map[string]time.Time

	active  int
	waiting int

	// cancel stops the most recent run, for ModeRestart.
	cancel context.CancelFunc

	// serial holds a queued run while another is in flight.
	serial sync.Mutex

	// wg tracks in-flight runs so shutdown can wait them out instead of
	// abandoning them mid-service-call.
	wg sync.WaitGroup
}

func newRunner(policy Policy, clock internal.Clock) *runner {
	return &runner{policy: policy, clock: clock, lastRan: map[string]time.Time{}}
}

// withClock points the runner at the app's clock. Conditions already read it,
// and a throttle measured against a different clock than the conditions it
// gates is not testable and not coherent.
func (r *runner) withClock(clock internal.Clock) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clock = clock
}

// run admits a trigger under the policy and reports whether it was accepted.
// The work happens on its own goroutine, so the caller, which is a dispatch
// worker, is never held by a slow automation.
func (r *runner) run(parent context.Context, key string, fn func(context.Context)) bool {
	r.mu.Lock()
	now := r.clock.Now()

	// Admission and the stamp that records it are one critical section, so two
	// triggers cannot both read the same lastRan and both decide they are past
	// the window.
	if last, seen := r.lastRan[key]; r.policy.Throttle > 0 && seen &&
		now.Sub(last) < r.policy.Throttle {
		r.mu.Unlock()
		return false
	}

	switch r.policy.Mode {
	case ModeSingle:
		if r.active > 0 {
			r.mu.Unlock()
			return false
		}
	case ModeRestart:
		if r.cancel != nil {
			r.cancel()
		}
	case ModeQueued:
		if r.waiting >= r.policy.limit() {
			r.mu.Unlock()
			return false
		}
		r.waiting++
	case ModeParallel:
		if r.active >= r.policy.limit() {
			r.mu.Unlock()
			return false
		}
	}

	r.lastRan[key] = now
	r.active++

	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel

	queued := r.policy.Mode == ModeQueued
	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()

		if queued {
			// Held for the duration of the run, which is what keeps queued runs
			// from overlapping.
			r.serial.Lock()
			defer r.serial.Unlock()

			r.mu.Lock()
			r.waiting--
			r.mu.Unlock()
		}

		defer r.finish()
		fn(ctx)
	}()

	return true
}

func (r *runner) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
}

// wait blocks until every admitted run has finished.
func (r *runner) wait() { r.wg.Wait() }
