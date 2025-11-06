package ha

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(d int) time.Time {
	return time.Date(2025, time.November, d, 0, 0, 0, 0, time.Local)
}

func nothingEntity(*Service, StateReader, EntityData) {}

func nothingEvent(*Service, StateReader, EventData) {}

// exceptionDatesOf builds each automation kind through its own builder and
// returns the dates that landed on it, so one table can cover all four.
var exceptionDatesOf = map[string]func(calls [][]time.Time) []time.Time{
	"DailySchedule": func(calls [][]time.Time) []time.Time {
		b := NewDailySchedule().Call(nothing).At("12:00")
		for _, c := range calls {
			b = b.ExceptionDates(c[0], c[1:]...)
		}
		return b.Build().exceptionDates
	},
	"Interval": func(calls [][]time.Time) []time.Time {
		b := NewInterval().Call(nothing).Every("1h")
		for _, c := range calls {
			b = b.ExceptionDates(c[0], c[1:]...)
		}
		return b.Build().exceptionDates
	},
	"EntityListener": func(calls [][]time.Time) []time.Time {
		b := NewEntityListener().EntityIds("light.kitchen").Call(nothingEntity)
		for _, c := range calls {
			b = b.ExceptionDates(c[0], c[1:]...)
		}
		return b.Build().exceptionDates
	},
	"EventListener": func(calls [][]time.Time) []time.Time {
		b := NewEventListener().EventTypes("call_service").Call(nothingEvent)
		for _, c := range calls {
			b = b.ExceptionDates(c[0], c[1:]...)
		}
		return b.Build().exceptionDates
	},
}

func TestExceptionDatesAccumulate(t *testing.T) {
	for name, build := range exceptionDatesOf {
		t.Run(name, func(t *testing.T) {
			t.Run("a single call keeps its dates in order", func(t *testing.T) {
				got := build([][]time.Time{{day(1), day(2), day(3)}})
				assert.Equal(t, []time.Time{day(1), day(2), day(3)}, got)
			})

			t.Run("repeated calls accumulate", func(t *testing.T) {
				got := build([][]time.Time{{day(1)}, {day(2)}, {day(3)}})
				assert.Equal(t, []time.Time{day(1), day(2), day(3)}, got,
					"a later call must not discard the dates set by an earlier one")
			})

			t.Run("the required argument comes first", func(t *testing.T) {
				got := build([][]time.Time{{day(1), day(2)}})
				require.Len(t, got, 2)
				assert.Equal(t, day(1), got[0])
			})
		})
	}
}

func TestExceptionDatesDoNotAliasTheCallerSlice(t *testing.T) {
	rest := []time.Time{day(2), day(3)}

	s := NewDailySchedule().Call(nothing).At("12:00").ExceptionDates(day(1), rest...).Build()
	require.Equal(t, []time.Time{day(1), day(2), day(3)}, s.exceptionDates)

	// Mutating the slice the caller passed must not reach the built schedule.
	rest[0] = day(9)

	assert.Equal(t, []time.Time{day(1), day(2), day(3)}, s.exceptionDates)
}

func TestOnlyOnDatesAccumulate(t *testing.T) {
	s := NewDailySchedule().Call(nothing).At("12:00").
		OnlyOnDates(day(1)).
		OnlyOnDates(day(2), day(3)).
		Build()

	assert.Equal(t, []time.Time{day(1), day(2), day(3)}, s.allowlistDates)
}
