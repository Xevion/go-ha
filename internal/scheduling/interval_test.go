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
		// With a zero epoch, NextTime should calculate from the last hour boundary.
		next := trigger.NextTime(now)
		expected := time.Date(2024, 7, 25, 13, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, *next)
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
