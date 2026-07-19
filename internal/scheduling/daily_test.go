package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedTimeTrigger_NextTime(t *testing.T) {
	// Carries a second and a nanosecond so anything inherited from now rather
	// than built from Hour and Minute shows up in the comparison.
	now := time.Date(2025, 8, 2, 10, 0, 37, 412000000, time.Local)

	tests := []struct {
		name     string
		hour     int
		minute   int
		expected time.Time
	}{
		{
			name:     "later today",
			hour:     14,
			minute:   30,
			expected: time.Date(2025, 8, 2, 14, 30, 0, 0, time.Local),
		},
		{
			name:     "already passed, so tomorrow",
			hour:     8,
			minute:   0,
			expected: time.Date(2025, 8, 3, 8, 0, 0, 0, time.Local),
		},
		{
			name:     "this minute is already in progress",
			hour:     10,
			minute:   0,
			expected: time.Date(2025, 8, 3, 10, 0, 0, 0, time.Local),
		},
		{
			name:     "the very next minute",
			hour:     10,
			minute:   1,
			expected: time.Date(2025, 8, 2, 10, 1, 0, 0, time.Local),
		},
		{
			name:     "midnight rolls over",
			hour:     0,
			minute:   0,
			expected: time.Date(2025, 8, 3, 0, 0, 0, 0, time.Local),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &FixedTimeTrigger{
				Hour:   tt.hour,
				Minute: tt.minute,
			}

			result := trigger.NextTime(now)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, *result)
		})
	}
}
