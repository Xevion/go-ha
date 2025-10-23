package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deliberately carries a non-zero second and a late hour, so a result that
// leaked either from the clock rather than the parsed string is visible.
var parseBase = time.Date(2025, time.October, 22, 21, 52, 7, 0, time.Local)

func TestParseTime(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		hour   int
		minute int
	}{
		{"midnight", "00:00", 0, 0},
		{"noon", "12:00", 12, 0},
		{"last minute of the day", "23:59", 23, 59},
		{"leading zero hour", "09:05", 9, 5},
		{"before the clock's own time", "06:30", 6, 30},
		{"after the clock's own time", "22:15", 22, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTime(NewFakeClock(parseBase), tt.input).StdTime()

			assert.Equal(t, parseBase.Year(), got.Year())
			assert.Equal(t, parseBase.Month(), got.Month())
			assert.Equal(t, parseBase.Day(), got.Day(), "the day must come from the clock")
			assert.Equal(t, tt.hour, got.Hour())
			assert.Equal(t, tt.minute, got.Minute())
			assert.Zero(t, got.Second(), "the clock's seconds must not leak through")
			assert.Zero(t, got.Nanosecond())
		})
	}
}

func TestParseTimeAnchorsToTheClock(t *testing.T) {
	clock := NewFakeClock(parseBase)

	before := ParseTime(clock, "08:00").StdTime()
	clock.Advance(48 * time.Hour)
	after := ParseTime(clock, "08:00").StdTime()

	assert.Equal(t, 48*time.Hour, after.Sub(before), "the same input must follow the clock's day")
}

func TestParseTimeRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"not a time", "abc"},
		{"hour out of range", "25:00"},
		{"minute out of range", "12:60"},
		{"missing separator", "1200"},
		{"seconds supplied", "12:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				require.NotNil(t, r, "malformed input must panic")

				err, ok := r.(error)
				require.True(t, ok, "panic value must be an error, got %T", r)
				assert.Contains(t, err.Error(), tt.input, "the message must name the offending string")
				assert.Contains(t, err.Error(), "HH:MM", "the message must state the expected format")
			}()

			ParseTime(NewFakeClock(parseBase), tt.input)
		})
	}
}
