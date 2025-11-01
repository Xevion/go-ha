package ha

import (
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
func (s *scheduler) add(trigger scheduling.Trigger, run func()) {
	next := trigger.NextTime(s.clock.Now())
	s.push(&scheduledEntry{trigger: trigger, run: run, fireAt: *next})
}

func (s *scheduler) push(entry *scheduledEntry) {
	s.queue.Put(Item{
		Value:    entry,
		Priority: float64(entry.fireAt.Unix()),
	})
}

// pop removes and returns the entry due soonest, blocking until one exists.
func (s *scheduler) pop() *scheduledEntry {
	items, err := s.queue.Get(1)
	if err != nil || len(items) == 0 {
		return nil
	}
	return items[0].(Item).Value.(*scheduledEntry)
}

// requeue puts an entry back for its following occurrence.
func (s *scheduler) requeue(entry *scheduledEntry) {
	next := entry.trigger.NextTime(s.clock.Now())
	entry.fireAt = *next
	s.push(entry)
}

func (s *scheduler) len() int {
	return s.queue.Len()
}
