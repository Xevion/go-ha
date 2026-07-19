package core

import (
	"testing"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var schedulerBase = time.Date(2025, time.November, 1, 13, 47, 12, 0, time.Local)

func fixedAt(hour, minute int) scheduling.Trigger {
	return &scheduling.FixedTimeTrigger{Hour: hour, Minute: minute}
}

func noop() {}

func TestSchedulerAdd(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))
	assert.Zero(t, s.len())

	s.add(fixedAt(18, 0), noop)
	assert.Equal(t, 1, s.len())

	s.add(fixedAt(9, 0), noop)
	assert.Equal(t, 2, s.len())
}

func TestSchedulerPopsInAscendingOrder(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	// Registered out of order, and 09:00 has already passed today so it belongs
	// after tomorrow's earlier slots rather than first.
	s.add(fixedAt(18, 0), noop)
	s.add(fixedAt(9, 0), noop)
	s.add(fixedAt(14, 30), noop)

	var got []time.Time
	for s.len() > 0 {
		entry := s.pop()
		require.NotNil(t, entry)
		got = append(got, entry.fireAt)
	}

	require.Len(t, got, 3)
	assert.Equal(t, time.Date(2025, time.November, 1, 14, 30, 0, 0, time.Local), got[0])
	assert.Equal(t, time.Date(2025, time.November, 1, 18, 0, 0, 0, time.Local), got[1])
	assert.Equal(t, time.Date(2025, time.November, 2, 9, 0, 0, 0, time.Local), got[2])
}

func TestSchedulerRequeueAdvancesToTheFollowingDay(t *testing.T) {
	clock := internal.NewFakeClock(schedulerBase)
	s := newScheduler(clock)
	s.add(fixedAt(18, 0), noop)

	entry := s.pop()
	require.NotNil(t, entry)
	assert.Equal(t, time.Date(2025, time.November, 1, 18, 0, 0, 0, time.Local), entry.fireAt)

	clock.Set(entry.fireAt)
	s.requeue(entry)

	requeued := s.pop()
	require.NotNil(t, requeued)
	assert.Equal(t, time.Date(2025, time.November, 2, 18, 0, 0, 0, time.Local), requeued.fireAt)
}

func TestSchedulerRequeueAdvancesFromTheFireTime(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))
	s.add(fixedAt(18, 0), noop)

	entry := s.pop()
	require.NotNil(t, entry)
	first := entry.fireAt

	// The clock has not moved past the slot yet. Requeueing must still advance to
	// the following occurrence rather than resolving the same one again.
	require.True(t, s.requeue(entry))

	second := s.pop()
	require.NotNil(t, second)

	assert.True(t, second.fireAt.After(first), "requeue must advance past the slot it just ran")

	// Nov 2 is the DST fall back, so the following occurrence is 25 absolute
	// hours out while still landing on 18:00 local. Assert the wall clock, not
	// the elapsed duration.
	assert.Equal(t, first.Day()+1, second.fireAt.Day())
	assert.Equal(t, 18, second.fireAt.Hour())
	assert.Zero(t, second.fireAt.Minute())
}

func TestSchedulerRunsTheEntryCallback(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	fired := false
	s.add(fixedAt(18, 0), func() { fired = true })

	entry := s.pop()
	require.NotNil(t, entry)
	entry.run()

	assert.True(t, fired, "the callback registered with add must be the one queued")
}

func TestSchedulerRunDue(t *testing.T) {
	t.Run("fires nothing before the first slot", func(t *testing.T) {
		clock := internal.NewFakeClock(schedulerBase)
		s := newScheduler(clock)

		var fired []string
		s.add(fixedAt(14, 0), func() { fired = append(fired, "14:00") })

		assert.Zero(t, s.runDue(clock.Now()))
		assert.Empty(t, fired)
		assert.Equal(t, 1, s.len())
	})

	t.Run("catches up across several missed slots in order", func(t *testing.T) {
		clock := internal.NewFakeClock(schedulerBase)
		s := newScheduler(clock)

		var fired []string
		s.add(fixedAt(16, 0), func() { fired = append(fired, "16:00") })
		s.add(fixedAt(14, 0), func() { fired = append(fired, "14:00") })
		s.add(fixedAt(15, 0), func() { fired = append(fired, "15:00") })

		// One jump past all three, as a suspended process would see.
		clock.Advance(3 * time.Hour)

		assert.Equal(t, 3, s.runDue(clock.Now()))
		assert.Equal(t, []string{"14:00", "15:00", "16:00"}, fired)
		assert.Equal(t, 3, s.len(), "each schedule must be left queued exactly once")
	})

	t.Run("fires a slot landed on exactly", func(t *testing.T) {
		clock := internal.NewFakeClock(schedulerBase)
		s := newScheduler(clock)

		fired := 0
		s.add(fixedAt(14, 0), func() { fired++ })

		clock.Set(time.Date(2025, time.November, 1, 14, 0, 0, 0, time.Local))

		assert.Equal(t, 1, s.runDue(clock.Now()))
		assert.Equal(t, 1, fired)
	})

	t.Run("a repeated pass does not fire again", func(t *testing.T) {
		clock := internal.NewFakeClock(schedulerBase)
		s := newScheduler(clock)

		fired := 0
		s.add(fixedAt(14, 0), func() { fired++ })

		clock.Advance(time.Hour)
		require.Equal(t, 1, s.runDue(clock.Now()))
		assert.Zero(t, s.runDue(clock.Now()), "the requeued slot is a day out")
		assert.Equal(t, 1, fired)
	})
}

func TestSchedulerPeekLeavesTheEntryQueued(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))
	s.add(fixedAt(18, 0), noop)

	first := s.peek()
	require.NotNil(t, first)
	assert.Equal(t, 1, s.len())

	second := s.peek()
	require.NotNil(t, second)
	assert.Equal(t, first.fireAt, second.fireAt)
	assert.Equal(t, 1, s.len())
}

// oneShotTrigger fires once and then reports no further occurrence, which is
// what a sun trigger does when it runs into a polar night.
type oneShotTrigger struct {
	at    time.Time
	spent bool
}

func (t *oneShotTrigger) NextTime(time.Time) *time.Time {
	if t.spent {
		return nil
	}
	t.spent = true
	return &t.at
}

func (t *oneShotTrigger) Hash() uint64 { return 1 }

func TestSchedulerDoesNotBlockOnAnEmptyQueue(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.Nil(t, s.pop())
		assert.Nil(t, s.peek())
		assert.Zero(t, s.runDue(schedulerBase))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pop, peek or runDue blocked on an empty queue")
	}
}

func TestSchedulerRunDueSurvivesAnExhaustedTrigger(t *testing.T) {
	clock := internal.NewFakeClock(schedulerBase)
	s := newScheduler(clock)

	fired := 0
	require.True(t, s.add(&oneShotTrigger{at: schedulerBase.Add(time.Hour)}, func() { fired++ }))

	clock.Advance(2 * time.Hour)

	type result struct{ ran int }
	done := make(chan result, 1)
	go func() { done <- result{ran: s.runDue(clock.Now())} }()

	select {
	case got := <-done:
		assert.Equal(t, 1, got.ran)
		assert.Equal(t, 1, fired)
		assert.Zero(t, s.len(), "the exhausted trigger must leave the queue empty")
	case <-time.After(5 * time.Second):
		t.Fatal("runDue blocked after an exhausted trigger emptied the queue")
	}
}

func TestSchedulerKeepsDistinctEntriesAtTheSameInstant(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	fired := make([]string, 0, 2)
	require.True(t, s.add(fixedAt(7, 0), func() { fired = append(fired, "first") }))
	require.True(t, s.add(fixedAt(7, 0), func() { fired = append(fired, "second") }))

	require.Equal(t, 2, s.len(), "two schedules may legitimately want the same moment")

	for s.len() > 0 {
		entry := s.pop()
		require.NotNil(t, entry)
		entry.run()
	}

	assert.ElementsMatch(t, []string{"first", "second"}, fired)
}
