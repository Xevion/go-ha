package core

import (
	"sync"
	"time"
)

// delayedTrigger is implemented by triggers that fire only once a condition has
// held for some time, rather than at the instant it becomes true.
type delayedTrigger interface {
	holdFor() time.Duration
	concerns(ev Event) bool
}

// pendingRuns holds the timers for triggers waiting out a For duration.
//
// Timers are keyed by entity. One automation can watch many entities, and a
// single timer between them means the second entity to change cancels the
// first entity's pending run.
type pendingRuns struct {
	mu     sync.Mutex
	timers map[string]*time.Timer

	// gen numbers the waits armed for each entity. Stop cannot recall a timer
	// whose callback has already begun, so a superseded callback would
	// otherwise delete the map entry belonging to its own replacement, leaving
	// that replacement untracked and beyond the reach of disarm and stop.
	gen map[string]uint64

	closed bool
}

func newPendingRuns() *pendingRuns {
	return &pendingRuns{
		timers: map[string]*time.Timer{},
		gen:    map[string]uint64{},
	}
}

// arm schedules run for this entity, replacing any wait already in progress.
func (p *pendingRuns) arm(entityID string, d time.Duration, run func()) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	if existing, ok := p.timers[entityID]; ok {
		existing.Stop()
	}

	p.gen[entityID]++
	mine := p.gen[entityID]

	p.timers[entityID] = time.AfterFunc(d, func() {
		p.mu.Lock()
		// Only the newest wait for this entity may act. An older one whose
		// callback started before it could be stopped finds a newer generation
		// here and retires quietly.
		current := p.gen[entityID] == mine && !p.closed
		if current {
			delete(p.timers, entityID)
		}
		p.mu.Unlock()

		if current {
			run()
		}
	})
}

// disarm cancels this entity's wait, because the state moved away before it
// elapsed.
func (p *pendingRuns) disarm(entityID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if timer, ok := p.timers[entityID]; ok {
		timer.Stop()
		delete(p.timers, entityID)
	}
	// Advanced even when no timer was found, so a callback already in flight
	// sees itself superseded and does not run.
	p.gen[entityID]++
}

// stop cancels every wait and refuses further ones, for shutdown.
func (p *pendingRuns) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	for id, timer := range p.timers {
		timer.Stop()
		delete(p.timers, id)
	}
}
