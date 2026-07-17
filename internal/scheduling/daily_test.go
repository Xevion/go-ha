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

func TestFixedTimeTrigger_Hash(t *testing.T) {
	tests := []struct {
		name     string
		hour     int
		minute   int
		expected uint64
	}{
		{
			name:     "basic time",
			hour:     12,
			minute:   30,
			expected: 0, // We'll check it's not zero
		},
		{
			name:     "midnight",
			hour:     0,
			minute:   0,
			expected: 0, // We'll check it's not zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &FixedTimeTrigger{
				Hour:   tt.hour,
				Minute: tt.minute,
			}

			hash := trigger.Hash()
			assert.NotZero(t, hash)
			assert.IsType(t, uint64(0), hash)
		})
	}

	// Test that different times produce different hashes
	trigger1 := &FixedTimeTrigger{Hour: 12, Minute: 30}
	trigger2 := &FixedTimeTrigger{Hour: 12, Minute: 31}
	trigger3 := &FixedTimeTrigger{Hour: 13, Minute: 30}

	hash1 := trigger1.Hash()
	hash2 := trigger2.Hash()
	hash3 := trigger3.Hash()

	assert.NotEqual(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
	assert.NotEqual(t, hash2, hash3)

	// Test that same times produce same hashes
	trigger4 := &FixedTimeTrigger{Hour: 12, Minute: 30}
	hash4 := trigger4.Hash()
	assert.Equal(t, hash1, hash4)
}
