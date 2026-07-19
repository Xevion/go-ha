package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalOn evaluates c with the clock parked on the given instant.
func evalOn(t *testing.T, c Condition, at time.Time) bool {
	t.Helper()

	clock := testClock()
	clock.Set(at)

	got, err := c.Eval(context.Background(), EvalContext{Clock: clock})
	require.NoError(t, err)
	return got
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestOnDatesMatchesTheCalendarDay(t *testing.T) {
	c := OnDates(date(2026, time.December, 25), date(2027, time.January, 1))

	// Matching is by day, so the time of day is irrelevant.
	assert.True(t, evalOn(t, c, time.Date(2026, time.December, 25, 17, 30, 0, 0, time.UTC)))
	assert.True(t, evalOn(t, c, date(2027, time.January, 1)))
	assert.False(t, evalOn(t, c, date(2026, time.December, 24)))
}

func TestNotOnDates(t *testing.T) {
	c := Not(OnDates(date(2026, time.December, 25)))

	assert.False(t, evalOn(t, c, date(2026, time.December, 25)))
	assert.True(t, evalOn(t, c, date(2026, time.December, 26)))
}

func TestInDateRange(t *testing.T) {
	c := InDateRange(date(2026, time.July, 1), date(2026, time.August, 1))

	assert.False(t, evalOn(t, c, date(2026, time.June, 30)))
	assert.True(t, evalOn(t, c, date(2026, time.July, 1)), "start is included")
	assert.True(t, evalOn(t, c, date(2026, time.July, 15)))
	assert.False(t, evalOn(t, c, date(2026, time.August, 1)), "end is excluded")
}

func TestInDateRangeRejectsAnInvertedRange(t *testing.T) {
	c := InDateRange(date(2026, time.August, 1), date(2026, time.July, 1))

	v, ok := c.(interface{ validate() error })
	require.True(t, ok)
	assert.Error(t, v.validate(), "a range that ends before it starts can never hold")
}

func TestOnWeekdays(t *testing.T) {
	c := OnWeekdays(time.Saturday, time.Sunday)

	// 2026-07-19 is a Sunday, 2026-07-20 a Monday.
	assert.True(t, evalOn(t, c, date(2026, time.July, 19)))
	assert.False(t, evalOn(t, c, date(2026, time.July, 20)))
}
