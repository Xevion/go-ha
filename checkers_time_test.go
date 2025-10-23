package ha

import (
	"testing"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/types"
	"github.com/dromara/carbon/v2"
	"github.com/stretchr/testify/assert"
)

func atTime(hour, minute int) *internal.FakeClock {
	return internal.NewFakeClock(time.Date(2025, time.October, 22, hour, minute, 0, 0, time.Local))
}

func onDay(day int) time.Time {
	return time.Date(2025, time.October, day, 12, 0, 0, 0, time.Local)
}

func TestCheckExceptionDates(t *testing.T) {
	tests := []struct {
		name  string
		dates []time.Time
		fail  bool
	}{
		{"empty list never blocks", nil, false},
		{"today is excepted", []time.Time{onDay(22)}, true},
		{"yesterday is not today", []time.Time{onDay(21)}, false},
		{"tomorrow is not today", []time.Time{onDay(23)}, false},
		{"today among several", []time.Time{onDay(20), onDay(22), onDay(24)}, true},
		{"none match", []time.Time{onDay(20), onDay(24)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CheckExceptionDates(atTime(12, 0), tt.dates)
			assert.Equal(t, tt.fail, c.fail)
		})
	}
}

func TestCheckAllowlistDates(t *testing.T) {
	tests := []struct {
		name  string
		dates []time.Time
		fail  bool
	}{
		{"empty list imposes no restriction", nil, false},
		{"today is allowed", []time.Time{onDay(22)}, false},
		{"only another day is allowed", []time.Time{onDay(21)}, true},
		{"today among several", []time.Time{onDay(20), onDay(22)}, false},
		{"none match", []time.Time{onDay(20), onDay(24)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CheckAllowlistDates(atTime(12, 0), tt.dates)
			assert.Equal(t, tt.fail, c.fail)
		})
	}
}

func TestCheckExceptionRanges(t *testing.T) {
	start := time.Date(2025, time.October, 22, 10, 0, 0, 0, time.Local)
	end := time.Date(2025, time.October, 22, 14, 0, 0, 0, time.Local)
	ranges := []types.TimeRange{{Start: start, End: end}}

	tests := []struct {
		name string
		hour int
		fail bool
	}{
		{"before the range", 9, false},
		{"at the start boundary", 10, false},
		{"inside the range", 12, true},
		{"at the end boundary", 14, false},
		{"after the range", 15, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CheckExceptionRanges(atTime(tt.hour, 0), ranges)
			assert.Equal(t, tt.fail, c.fail)
		})
	}

	t.Run("empty list never blocks", func(t *testing.T) {
		c := CheckExceptionRanges(atTime(12, 0), nil)
		assert.False(t, c.fail)
	})
}

func TestCheckWithinTimeRange(t *testing.T) {
	tests := []struct {
		name   string
		start  string
		end    string
		hour   int
		minute int
		fail   bool
	}{
		{"plain range, inside", "09:00", "17:00", 12, 0, false},
		{"plain range, before", "09:00", "17:00", 8, 0, true},
		{"plain range, after", "09:00", "17:00", 18, 0, true},
		{"plain range, at start", "09:00", "17:00", 9, 0, false},

		// 23:00 to 07:00 spans midnight, so the window that contains 03:00
		// opened yesterday.
		{"overlap, early morning inside", "23:00", "07:00", 3, 0, false},
		{"overlap, mid afternoon outside", "23:00", "07:00", 15, 0, true},
		{"overlap, late evening inside", "23:00", "07:00", 23, 30, false},

		{"only start set, after it", "09:00", "", 12, 0, false},
		{"only start set, before it", "09:00", "", 8, 0, true},
		{"only end set, before it", "", "17:00", 12, 0, false},
		{"only end set, after it", "", "17:00", 18, 0, true},
		{"neither set", "", "", 12, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CheckWithinTimeRange(atTime(tt.hour, tt.minute), tt.start, tt.end)
			assert.Equal(t, tt.fail, c.fail)
		})
	}
}

func TestCheckThrottle(t *testing.T) {
	now := time.Date(2025, time.October, 23, 0, 26, 44, 0, time.Local)
	clock := internal.NewFakeClock(now)
	ago := func(d time.Duration) *carbon.Carbon {
		return carbon.CreateFromStdTime(now.Add(-d))
	}

	tests := []struct {
		name     string
		throttle time.Duration
		lastRan  *carbon.Carbon
		fail     bool
	}{
		{"no throttle set", 0, ago(time.Second), false},
		{"one second short of the period", time.Minute, ago(59 * time.Second), true},
		{"exactly one period back", time.Minute, ago(time.Minute), false},
		{"well inside the period", time.Minute, ago(time.Second), true},
		{"long past the period", time.Minute, ago(time.Hour), false},
		{"never ran", time.Minute, carbon.NewCarbon(now).StartOfCentury(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CheckThrottle(clock, tt.throttle, tt.lastRan)
			assert.Equal(t, tt.fail, c.fail)
		})
	}
}
