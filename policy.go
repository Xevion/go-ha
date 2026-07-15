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

// slot is the per-key execution state. Keying matters: one automation can watch
// many entities, and a single shared slot lets a busy entity consume the
// throttle window and cancel the runs of every other one.
type slot struct {
	lastRan time.Time
	active  int
	waiting int

	// cancel stops the most recent run, for ModeRestart.
	cancel context.CancelFunc

	// serial holds a queued run while another is in flight.
	serial sync.Mutex
}

// runner enforces one automation's policy.
type runner struct {
	policy Policy
	clock  internal.Clock

	mu    sync.Mutex
	slots map[string]*slot

	// wg tracks in-flight runs so shutdown can wait them out instead of
	// abandoning them mid-service-call.
	wg sync.WaitGroup
}

func newRunner(policy Policy, clock internal.Clock) *runner {
	return &runner{policy: policy, clock: clock, slots: map[string]*slot{}}
}

func (r *runner) slotFor(key string) *slot {
	s, ok := r.slots[key]
	if !ok {
		s = &slot{}
		r.slots[key] = s
	}
	return s
}

// run admits a trigger under the policy and reports whether it was accepted.
// The work happens on its own goroutine, so the caller, which is a dispatch
// worker, is never held by a slow automation.
func (r *runner) run(parent context.Context, key string, fn func(context.Context)) bool {
	now := r.clock.Now()

	r.mu.Lock()
	s := r.slotFor(key)

	// Admission and the stamp that records it are one critical section, so two
	// triggers cannot both read the same lastRan and both decide they are past
	// the window.
	if r.policy.Throttle > 0 && !s.lastRan.IsZero() && now.Sub(s.lastRan) < r.policy.Throttle {
		r.mu.Unlock()
		return false
	}

	switch r.policy.Mode {
	case ModeSingle:
		if s.active > 0 {
			r.mu.Unlock()
			return false
		}
	case ModeRestart:
		if s.cancel != nil {
			s.cancel()
		}
	case ModeQueued:
		if s.waiting >= r.policy.limit() {
			r.mu.Unlock()
			return false
		}
		s.waiting++
	case ModeParallel:
		if s.active >= r.policy.limit() {
			r.mu.Unlock()
			return false
		}
	}

	s.lastRan = now
	s.active++

	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel

	queued := r.policy.Mode == ModeQueued
	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer cancel()

		if queued {
			// Held for the duration of the run, which is what keeps queued runs
			// from overlapping.
			s.serial.Lock()
			defer s.serial.Unlock()

			r.mu.Lock()
			s.waiting--
			r.mu.Unlock()
		}

		defer r.finish(s)
		fn(ctx)
	}()

	return true
}

func (r *runner) finish(s *slot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s.active--
}

// wait blocks until every admitted run has finished.
func (r *runner) wait() { r.wg.Wait() }
