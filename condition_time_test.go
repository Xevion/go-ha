package ha

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalAt evaluates c with the clock parked at the given wall time.
func evalAt(t *testing.T, c Condition, hour, minute int) bool {
	t.Helper()

	clock := testClock()
	clock.Set(time.Date(2026, 3, 1, hour, minute, 0, 0, time.UTC))

	got, err := c.Eval(context.Background(), EvalContext{Clock: clock})
	require.NoError(t, err)
	return got
}

func TestTimeBetween(t *testing.T) {
	c := TimeBetween(TimeOfDay(9, 0), TimeOfDay(17, 0))

	tests := []struct {
		hour, minute int
		want         bool
		why          string
	}{
		{8, 59, false, "before the range"},
		{9, 0, true, "start is included"},
		{12, 0, true, "inside the range"},
		{16, 59, true, "just inside the end"},
		{17, 0, false, "end is excluded"},
		{23, 0, false, "after the range"},
	}

	for _, tt := range tests {
		t.Run(tt.why, func(t *testing.T) {
			assert.Equal(t, tt.want, evalAt(t, c, tt.hour, tt.minute))
		})
	}
}

// A range whose end is before its start runs through midnight, which is how
// "night lights from 23:00 to 07:00" is expressed.
func TestTimeBetweenCrossingMidnight(t *testing.T) {
	c := TimeBetween(TimeOfDay(23, 0), TimeOfDay(7, 0))

	tests := []struct {
		hour, minute int
		want         bool
		why          string
	}{
		{22, 59, false, "before the evening half"},
		{23, 0, true, "start of the evening half"},
		{23, 30, true, "inside the evening half"},
		{3, 0, true, "inside the morning half"},
		{6, 59, true, "just inside the morning end"},
		{7, 0, false, "end is excluded"},
		{12, 0, false, "the middle of the day is outside"},
	}

	for _, tt := range tests {
		t.Run(tt.why, func(t *testing.T) {
			assert.Equal(t, tt.want, evalAt(t, c, tt.hour, tt.minute))
		})
	}
}

func TestAfterTime(t *testing.T) {
	c := AfterTime(TimeOfDay(19, 0))

	assert.False(t, evalAt(t, c, 18, 59))
	assert.True(t, evalAt(t, c, 19, 0))
	assert.True(t, evalAt(t, c, 23, 59))
	assert.False(t, evalAt(t, c, 0, 30), "past midnight is a new day, not still after 19:00")
}

func TestBeforeTime(t *testing.T) {
	c := BeforeTime(TimeOfDay(7, 0))

	assert.True(t, evalAt(t, c, 0, 0))
	assert.True(t, evalAt(t, c, 6, 59))
	assert.False(t, evalAt(t, c, 7, 0))
	assert.False(t, evalAt(t, c, 20, 0))
}

func TestTimeOfDayRejectsImpossibleValues(t *testing.T) {
	for _, tt := range []struct{ hour, minute int }{
		{24, 0}, {-1, 0}, {0, 60}, {0, -1},
	} {
		c := TimeBetween(TimeOfDay(tt.hour, tt.minute), TimeOfDay(12, 0))
		v, ok := c.(interface{ validate() error })
		require.True(t, ok)
		assert.ErrorIs(t, v.validate(), ErrInvalidTimeOfDay,
			"%02d:%02d is not a time", tt.hour, tt.minute)
	}
}

func TestValidTimeOfDayReportsNoError(t *testing.T) {
	c := TimeBetween(TimeOfDay(0, 0), TimeOfDay(23, 59))
	v := c.(interface{ validate() error })
	assert.NoError(t, v.validate())
}
