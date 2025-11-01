package ha

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

func TestSchedulerRunsTheEntryCallback(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	fired := false
	s.add(fixedAt(18, 0), func() { fired = true })

	entry := s.pop()
	require.NotNil(t, entry)
	entry.run()

	assert.True(t, fired, "the callback registered with add must be the one queued")
}

func TestSchedulerQueuesSunTriggers(t *testing.T) {
	s := newScheduler(internal.NewFakeClock(schedulerBase))

	builder := scheduling.NewSchedule()
	builder.OnSunset()
	spec, err := builder.Build()
	require.NoError(t, err)

	trigger, err := spec.Resolve(scheduling.Location{Latitude: 40.7128, Longitude: -74.0060})
	require.NoError(t, err)

	s.add(trigger, noop)

	entry := s.pop()
	require.NotNil(t, entry, "a sun trigger must reach the queue rather than being dropped")
	assert.False(t, entry.fireAt.IsZero())
}
