package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyFiresAtTheGivenTime(t *testing.T) {
	trig := Daily(TimeOfDay(19, 30))

	next, ok := trig.NextTime(time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local))
	require.True(t, ok)
	assert.Equal(t, 19, next.Hour())
	assert.Equal(t, 30, next.Minute())
	assert.Equal(t, 19, next.Day())
}

// The scheduler requeues by asking for the time after the one that just fired,
// so a trigger that returns that same instant would spin.
func TestDailyAdvancesToTheNextDay(t *testing.T) {
	trig := Daily(TimeOfDay(19, 30))

	first, ok := trig.NextTime(time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local))
	require.True(t, ok)

	second, ok := trig.NextTime(first)
	require.True(t, ok)
	assert.True(t, second.After(first))
	assert.Equal(t, 20, second.Day())
}

func TestEveryFiresOnTheInterval(t *testing.T) {
	trig := Every(15 * time.Minute)

	start := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	first, ok := trig.NextTime(start)
	require.True(t, ok)

	second, ok := trig.NextTime(first)
	require.True(t, ok)
	assert.Equal(t, 15*time.Minute, second.Sub(first))
}

func TestCronFiresOnTheExpression(t *testing.T) {
	trig := Cron("0 9 * * *")

	next, ok := trig.NextTime(time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local))
	require.True(t, ok)
	assert.Equal(t, 9, next.Hour())
	assert.Equal(t, 20, next.Day(), "9am has passed, so the next one is tomorrow")
}

// A malformed trigger reports itself rather than panicking, so the automation
// holding it can fail to build with a useful message.
func TestCronReportsAnInvalidExpression(t *testing.T) {
	trig := Cron("not a cron expression")

	v, ok := trig.(interface{ validate() error })
	require.True(t, ok)
	assert.Error(t, v.validate())

	_, fires := trig.NextTime(time.Now())
	assert.False(t, fires, "an invalid trigger must never fire")
}

func TestDailyReportsAnInvalidTime(t *testing.T) {
	trig := Daily(TimeOfDay(25, 0))

	v := trig.(interface{ validate() error })
	assert.ErrorIs(t, v.validate(), ErrInvalidTimeOfDay)

	_, fires := trig.NextTime(time.Now())
	assert.False(t, fires)
}

func TestEveryReportsAnInvalidInterval(t *testing.T) {
	trig := Every(0)

	v := trig.(interface{ validate() error })
	assert.Error(t, v.validate())
}
