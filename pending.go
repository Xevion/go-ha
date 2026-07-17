package ha

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
	closed bool
}

func newPendingRuns() *pendingRuns {
	return &pendingRuns{timers: map[string]*time.Timer{}}
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

	p.timers[entityID] = time.AfterFunc(d, func() {
		p.mu.Lock()
		delete(p.timers, entityID)
		stopped := p.closed
		p.mu.Unlock()

		// Shutdown can land between the timer firing and this point, and a
		// callback that starts then has nowhere to send its service calls.
		if !stopped {
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
