package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntervalTrigger(t *testing.T) {
	t.Run("valid single interval", func(t *testing.T) {
		trigger, err := NewIntervalTrigger(time.Hour)
		require.NoError(t, err)
		assert.NotNil(t, trigger)
		assert.Equal(t, []time.Duration{time.Hour}, trigger.intervals)
		assert.Equal(t, time.Hour, trigger.totalDuration)
		assert.True(t, trigger.epoch.IsZero())
	})

	t.Run("valid multiple intervals", func(t *testing.T) {
		trigger, err := NewIntervalTrigger(time.Hour, 30*time.Minute)
		require.NoError(t, err)
		assert.NotNil(t, trigger)
		assert.Equal(t, []time.Duration{time.Hour, 30 * time.Minute}, trigger.intervals)
		assert.Equal(t, time.Hour+30*time.Minute, trigger.totalDuration)
		assert.True(t, trigger.epoch.IsZero())
	})

	t.Run("invalid zero interval", func(t *testing.T) {
		_, err := NewIntervalTrigger(time.Hour, 0)
		assert.Error(t, err)
	})

	t.Run("invalid negative interval", func(t *testing.T) {
		_, err := NewIntervalTrigger(time.Hour, -time.Minute)
		assert.Error(t, err)
	})

	t.Run("first interval is invalid if zero", func(t *testing.T) {
		_, err := NewIntervalTrigger(0)
		assert.Error(t, err)
	})
}

func TestIntervalTrigger_NextTime(t *testing.T) {
	// A known time for predictable tests
	now := time.Date(2024, 7, 25, 12, 0, 0, 0, time.UTC)

	t.Run("single interval no epoch", func(t *testing.T) {
		trigger, _ := NewIntervalTrigger(time.Hour)
		// With a zero epoch, NextTime calculates from the last hour boundary and
		// reports it in local time, so compare the instant rather than the fields.
		next := trigger.NextTime(now)
		expected := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		assert.True(t, expected.Equal(*next), "expected %s, got %s", expected, *next)
	})

	t.Run("single interval with aligned epoch", func(t *testing.T) {
		trigger, _ := NewIntervalTrigger(time.Hour)
		// Epoch is on an hour boundary relative to the Unix epoch, so it's not modified by WithEpoch.
		epoch := time.Date(2024, 7, 25, 0, 0, 0, 0, time.UTC)
		trigger.WithEpoch(epoch)
		next := trigger.NextTime(now)
		expected := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, *next)
	})

	t.Run("multiple intervals", func(t *testing.T) {
		trigger, _ := NewIntervalTrigger(time.Hour, 30*time.Minute) // total 1.5h
		epoch := time.Date(2024, 7, 25, 0, 0, 0, 0, time.UTC)
		trigger.WithEpoch(epoch)
		// now = 12:00. epoch = 00:00. duration = 12h.
		// cycles = 12h / 1.5h = 8.
		// currentCycleStart = 00:00 + 8 * 1.5h = 12:00.
		// 1. 12:00 + 1h = 13:00. This is after now, so it's the next time.
		next := trigger.NextTime(now)
		expected := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, *next)

		// Test the time after that
		now2 := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		// currentCycleStart is still 12:00.
		// 1. 12:00 + 1h = 13:00. Not after now2.
		// 2. 13:00 + 30m = 13:30. This is after now2.
		next2 := trigger.NextTime(now2)
		expected2 := time.Date(2024, 7, 25, 13, 30, 0, 0, time.UTC)
		assert.Equal(t, expected2, *next2)
	})

	t.Run("now before epoch", func(t *testing.T) {
		trigger, _ := NewIntervalTrigger(time.Hour)
		epoch := time.Date(2024, 7, 26, 0, 0, 0, 0, time.UTC)
		trigger.WithEpoch(epoch)
		next := trigger.NextTime(now)
		expected := time.Date(2024, 7, 26, 1, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, *next)
	})

	t.Run("now is exactly on a trigger time", func(t *testing.T) {
		trigger, _ := NewIntervalTrigger(time.Hour)
		epoch := time.Date(2024, 7, 25, 0, 0, 0, 0, time.UTC)
		trigger.WithEpoch(epoch)
		nowOnTrigger := time.Date(2024, 7, 25, 12, 0, 0, 0, time.UTC)
		// The next trigger should be the following one.
		next := trigger.NextTime(nowOnTrigger)
		expected := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, *next)
	})

}

func TestIntervalTrigger_NextTimeAcrossDSTFallBack(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	// 2025-11-02 is the fall back. 01:00 through 02:00 runs on CDT, then the
	// clock returns to 01:00 on CST and the wall clock hour repeats.
	epoch := time.Date(2025, 11, 2, 1, 0, 0, 0, chicago)
	trigger, err := NewIntervalTrigger(30 * time.Minute)
	require.NoError(t, err)
	trigger.WithEpoch(epoch)

	cursor := time.Date(2025, 11, 2, 0, 30, 0, 0, chicago)
	got := make([]time.Time, 0, 5)
	for i := 0; i < 5; i++ {
		next := trigger.NextTime(cursor)
		require.NotNil(t, next)
		got = append(got, *next)
		cursor = *next
	}

	for i := 1; i < len(got); i++ {
		assert.Equal(t, 30*time.Minute, got[i].Sub(got[i-1]),
			"the interval must stay 30 real minutes wide across the transition")
	}

	// The repeated wall clock reading is correct, these are distinct instants.
	assert.Equal(t, "01:30", got[0].Format("15:04"))
	assert.Equal(t, "01:00", got[1].Format("15:04"))
	assert.NotEqual(t, got[0].Unix(), got[1].Unix())
}

func TestIntervalTrigger_NextTimeReportsLocalTime(t *testing.T) {
	trigger, err := NewIntervalTrigger(30 * time.Minute)
	require.NoError(t, err)

	now := time.Date(2025, 11, 2, 0, 30, 0, 0, time.Local)
	next := trigger.NextTime(now)
	require.NotNil(t, next)

	assert.Equal(t, time.Local.String(), next.Location().String(),
		"an interval with no epoch must report local time, as the fixed time and sun triggers do")
}

func TestIntervalTrigger_Hash(t *testing.T) {
	t.Run("stable hash for same configuration", func(t *testing.T) {
		trigger1, _ := NewIntervalTrigger(time.Hour, 30*time.Minute)
		trigger2, _ := NewIntervalTrigger(time.Hour, 30*time.Minute)
		assert.Equal(t, trigger1.Hash(), trigger2.Hash())
	})

	t.Run("hash changes with interval", func(t *testing.T) {
		trigger1, _ := NewIntervalTrigger(time.Hour, 30*time.Minute)
		trigger2, _ := NewIntervalTrigger(time.Hour, 31*time.Minute)
		assert.NotEqual(t, trigger1.Hash(), trigger2.Hash())
	})

	t.Run("hash changes with epoch", func(t *testing.T) {
		trigger1, _ := NewIntervalTrigger(time.Hour)
		trigger1.WithEpoch(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		trigger2, _ := NewIntervalTrigger(time.Hour)
		trigger2.WithEpoch(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))

		assert.NotEqual(t, trigger1.Hash(), trigger2.Hash())
	})
}
