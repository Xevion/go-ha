package ha

import (
	"testing"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nothing(*Service, StateReader) {}

func TestScheduleCron(t *testing.T) {
	t.Run("a five field expression resolves to a trigger", func(t *testing.T) {
		s := NewDailySchedule().Call(nothing).Cron("0 7 * * *").Build()

		require.NoError(t, s.specErr)
		require.NotNil(t, s.spec)

		trigger, err := s.spec.Resolve(scheduling.Location{})
		require.NoError(t, err)

		next := trigger.NextTime(schedulerBase)
		require.NotNil(t, next)
		assert.Equal(t, time.Date(2025, time.November, 2, 7, 0, 0, 0, time.Local), *next)
	})

	t.Run("a step expression resolves to the next interval", func(t *testing.T) {
		s := NewDailySchedule().Call(nothing).Cron("*/15 * * * *").Build()
		require.NoError(t, s.specErr)

		trigger, err := s.spec.Resolve(scheduling.Location{})
		require.NoError(t, err)

		next := trigger.NextTime(schedulerBase)
		require.NotNil(t, next)
		assert.Equal(t, time.Date(2025, time.November, 1, 14, 0, 0, 0, time.Local), *next)
	})

	t.Run("an invalid expression is an error, not a panic", func(t *testing.T) {
		var s DailySchedule

		assert.NotPanics(t, func() {
			s = NewDailySchedule().Call(nothing).Cron("not a cron expression").Build()
		})

		assert.Error(t, s.specErr)
		assert.Nil(t, s.spec)
	})
}

func TestScheduleCronReachesTheScheduler(t *testing.T) {
	s := NewDailySchedule().Call(nothing).Cron("0 7 * * *").Build()
	require.NoError(t, s.specErr)

	trigger, err := s.spec.Resolve(scheduling.Location{})
	require.NoError(t, err)

	sched := newScheduler(internal.NewFakeClock(schedulerBase))
	require.True(t, sched.add(trigger, noop))

	entry := sched.peek()
	require.NotNil(t, entry)
	assert.Equal(t, 7, entry.fireAt.Hour())
}
