package ha

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Workiva/go-datastructures/queue"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/scheduling"
)

// scheduledEntry pairs a trigger with the callback to run when it fires, and
// remembers the instant it is currently queued for.
type scheduledEntry struct {
	trigger scheduling.Trigger
	run     func()
	fireAt  time.Time
}

// scheduler orders triggers by their next fire time. It needs nothing but a
// Clock, so queue ordering and requeue arithmetic can be exercised without a
// connection, an HTTP client or a context.
type scheduler struct {
	// mu makes composite operations atomic against each other. refresh drains
	// the whole queue and refills it, and without this the run loop can peek
	// into that window, see an empty queue, and conclude there is nothing left
	// to do ever again.
	mu    sync.Mutex
	queue *queue.PriorityQueue
	clock internal.Clock
}

func newScheduler(clock internal.Clock) *scheduler {
	return &scheduler{
		queue: queue.NewPriorityQueue(100, false),
		clock: clock,
	}
}

// add queues trigger for its first fire time after the clock's current instant.
// A trigger with no next occurrence is reported and dropped.
func (s *scheduler) add(trigger scheduling.Trigger, run func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := trigger.NextTime(s.clock.Now())
	if next == nil {
		slog.Warn("Trigger has no next occurrence, not scheduling", "trigger", trigger)
		return false
	}

	s.push(&scheduledEntry{trigger: trigger, run: run, fireAt: *next})
	return true
}

func (s *scheduler) push(entry *scheduledEntry) {
	s.queue.Put(Item{
		Value:    entry,
		Priority: float64(entry.fireAt.Unix()),
	})
}

// pop removes and returns the entry due soonest, or nil when nothing is queued.
func (s *scheduler) pop() *scheduledEntry {
	// Get blocks until something is queued, and nothing else ever will be once
	// the run loop owns the scheduler, so it must never be called unguarded.
	if s.queue.Empty() {
		return nil
	}

	items, err := s.queue.Get(1)
	if err != nil || len(items) == 0 {
		return nil
	}
	return items[0].(Item).Value.(*scheduledEntry)
}

// requeue puts an entry back for its following occurrence, or drops it when the
// trigger has none left.
func (s *scheduler) requeue(entry *scheduledEntry) bool {
	next := entry.trigger.NextTime(entry.fireAt)
	if next == nil {
		slog.Warn("Trigger has no further occurrence, dropping", "trigger", entry.trigger)
		return false
	}

	entry.fireAt = *next
	s.push(entry)
	return true
}

// peek returns the entry due soonest without removing it, or nil when nothing
// is queued.
func (s *scheduler) peek() *scheduledEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peekLocked()
}

// nextFireAt reports when the soonest entry is due. It returns the time rather
// than the entry, because a caller reading fireAt off an entry does so outside
// the lock, where refresh may be rewriting it.
func (s *scheduler) nextFireAt() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.peekLocked()
	if entry == nil {
		return time.Time{}, false
	}
	return entry.fireAt, true
}

func (s *scheduler) peekLocked() *scheduledEntry {
	item := s.queue.Peek()
	if item == nil {
		return nil
	}
	return item.(Item).Value.(*scheduledEntry)
}

// runDue fires every entry due at or before now, requeueing each, and reports
// how many ran. A process suspended across several slots catches up here rather
// than losing them.
func (s *scheduler) runDue(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	fired := 0
	for {
		entry := s.pop()
		if entry == nil {
			return fired
		}

		if entry.fireAt.After(now) {
			s.push(entry)
			return fired
		}

		entry.run()
		s.requeue(entry)
		fired++
	}
}

func (s *scheduler) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Len()
}

// dynamicTrigger is implemented by triggers whose times move on their own,
// rather than only advancing when the trigger fires. A sun trigger reads its
// times from Home Assistant, which republishes them daily.
type dynamicTrigger interface {
	dynamic() bool
}

// refresh re-derives the fire time of every dynamic entry and reports how many
// moved.
//
// Without this a sun trigger could never track Home Assistant. It is asked for
// its next time immediately after firing, microseconds later, and the updated
// attribute has not crossed the network yet, so it would answer from the value
// that just expired every single time.
func (s *scheduler) refresh(now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.queue.Empty() {
		return 0
	}

	items, err := s.queue.Get(s.queue.Len())
	if err != nil {
		return 0
	}

	moved := 0
	for _, item := range items {
		entry := item.(Item).Value.(*scheduledEntry)

		// An entry already due is about to run. Re-deriving it here would push
		// it past now and skip that occurrence entirely.
		if entry.fireAt.After(now) {
			if dyn, ok := entry.trigger.(dynamicTrigger); ok && dyn.dynamic() {
				if next := entry.trigger.NextTime(now); next != nil && !next.Equal(entry.fireAt) {
					entry.fireAt = *next
					moved++
				}
			}
		}
		s.push(entry)
	}

	return moved
}

// run drives the scheduler until the context is cancelled. It fires everything
// due, then sleeps until the next entry falls due, a dynamic trigger moves, or
// the app shuts down.
func (s *scheduler) run(ctx context.Context, rescheduled <-chan struct{}, what string) {
	for {
		if ctx.Err() != nil {
			slog.Info("Scheduler shutting down", "kind", what)
			return
		}

		// Everything already due fires here, so a process suspended across
		// several slots catches up rather than losing them.
		s.runDue(s.clock.Now())

		// An empty queue is not the end. A trigger can retire and leave nothing
		// behind, an automation can be registered later, and a dynamic trigger
		// can come back with a time. Treating it as terminal meant one unlucky
		// peek during a refresh stopped every schedule for good.
		wait := time.Hour
		if next, ok := s.nextFireAt(); ok {
			wait = time.Until(next)
		}

		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-rescheduled:
			// A dynamic trigger moved, and the new time can be earlier than
			// the one being slept on, so the queue is re-read.
			timer.Stop()
		case <-ctx.Done():
			timer.Stop()
			slog.Info("Scheduler shutting down", "kind", what)
			return
		}
	}
}

// sameDate reports whether a and b fall on the same calendar day.
func sameDate(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
