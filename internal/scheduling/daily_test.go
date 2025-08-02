package scheduling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedTimeTrigger_NextTime(t *testing.T) {
	tests := []struct {
		name     string
		hour     int
		minute   int
		now      time.Time
		expected time.Time
	}{
		{
			name:     "same day trigger",
			hour:     14,
			minute:   30,
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 2, 14, 30, 0, 0, time.Local),
		},
		{
			name:     "next day trigger",
			hour:     8,
			minute:   0,
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 3, 8, 0, 0, 0, time.Local),
		},
		{
			name:     "exact time",
			hour:     10,
			minute:   0,
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 3, 10, 0, 0, 0, time.Local),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &FixedTimeTrigger{
				Hour:   tt.hour,
				Minute: tt.minute,
			}

			result := trigger.NextTime(tt.now)
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

func TestSunTrigger_NextTime(t *testing.T) {
	// Test with a known location (New York City)
	lat, lon := 40.7128, -74.0060

	tests := []struct {
		name     string
		sunset   bool
		offset   *time.Duration
		now      time.Time
		expected bool // whether we expect a result
	}{
		{
			name:     "sunrise without offset",
			sunset:   false,
			offset:   nil,
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: true,
		},
		{
			name:     "sunset without offset",
			sunset:   true,
			offset:   nil,
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: true,
		},
		{
			name:     "sunrise with positive offset",
			sunset:   false,
			offset:   func() *time.Duration { d := 30 * time.Minute; return &d }(),
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: true,
		},
		{
			name:     "sunset with negative offset",
			sunset:   true,
			offset:   func() *time.Duration { d := -1 * time.Hour; return &d }(),
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &SunTrigger{
				latitude:  lat,
				longitude: lon,
				sunset:    tt.sunset,
				offset:    tt.offset,
			}

			result := trigger.NextTime(tt.now)
			if tt.expected {
				require.NotNil(t, result)
				assert.False(t, result.IsZero())
			} else {
				// For polar regions or extreme dates, sun might not rise/set
				// This is acceptable behavior
			}
		})
	}
}

func TestSunTrigger_Hash(t *testing.T) {
	lat1, lon1 := 40.7128, -74.0060
	lat2, lon2 := 51.5074, -0.1278

	tests := []struct {
		name   string
		lat    float64
		lon    float64
		sunset bool
		offset *time.Duration
	}{
		{
			name:   "sunrise without offset",
			lat:    lat1,
			lon:    lon1,
			sunset: false,
			offset: nil,
		},
		{
			name:   "sunset with offset",
			lat:    lat1,
			lon:    lon1,
			sunset: true,
			offset: func() *time.Duration { d := 30 * time.Minute; return &d }(),
		},
		{
			name:   "different location",
			lat:    lat2,
			lon:    lon2,
			sunset: false,
			offset: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &SunTrigger{
				latitude:  tt.lat,
				longitude: tt.lon,
				sunset:    tt.sunset,
				offset:    tt.offset,
			}

			hash := trigger.Hash()
			assert.NotZero(t, hash)
			assert.IsType(t, uint64(0), hash)
		})
	}

	// Test that different configurations produce different hashes
	trigger1 := &SunTrigger{latitude: lat1, longitude: lon1, sunset: false, offset: nil}
	trigger2 := &SunTrigger{latitude: lat1, longitude: lon1, sunset: true, offset: nil}
	trigger3 := &SunTrigger{latitude: lat2, longitude: lon2, sunset: false, offset: nil}

	hash1 := trigger1.Hash()
	hash2 := trigger2.Hash()
	hash3 := trigger3.Hash()

	assert.NotEqual(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
	assert.NotEqual(t, hash2, hash3)

	// Test that same configurations produce same hashes
	trigger4 := &SunTrigger{latitude: lat1, longitude: lon1, sunset: false, offset: nil}
	hash4 := trigger4.Hash()
	assert.Equal(t, hash1, hash4)
}

func TestCompositeDailySchedule_NextTime(t *testing.T) {
	trigger1 := &FixedTimeTrigger{Hour: 8, Minute: 0}
	trigger2 := &FixedTimeTrigger{Hour: 12, Minute: 0}
	trigger3 := &FixedTimeTrigger{Hour: 18, Minute: 0}

	composite := &CompositeDailySchedule{
		triggers: []Trigger{trigger1, trigger2, trigger3},
	}

	now := time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local)
	result := composite.NextTime(now)

	require.NotNil(t, result)
	// Should return the earliest trigger after now (12:00)
	expected := time.Date(2025, 8, 2, 12, 0, 0, 0, time.Local)
	assert.Equal(t, expected, *result)
}

func TestCompositeDailySchedule_Hash(t *testing.T) {
	trigger1 := &FixedTimeTrigger{Hour: 8, Minute: 0}
	trigger2 := &FixedTimeTrigger{Hour: 12, Minute: 0}

	composite1 := &CompositeDailySchedule{
		triggers: []Trigger{trigger1, trigger2},
	}

	composite2 := &CompositeDailySchedule{
		triggers: []Trigger{trigger2, trigger1}, // Different order
	}

	composite3 := &CompositeDailySchedule{
		triggers: []Trigger{trigger1}, // Different number of triggers
	}

	hash1 := composite1.Hash()
	hash2 := composite2.Hash()
	hash3 := composite3.Hash()

	assert.NotZero(t, hash1)
	assert.NotZero(t, hash2)
	assert.NotZero(t, hash3)
	assert.IsType(t, uint64(0), hash1)

	// Different orders should produce different hashes
	assert.NotEqual(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
	assert.NotEqual(t, hash2, hash3)

	// Same configuration should produce same hash
	composite4 := &CompositeDailySchedule{
		triggers: []Trigger{trigger1, trigger2},
	}
	hash4 := composite4.Hash()
	assert.Equal(t, hash1, hash4)
}

func TestTriggerInterface(t *testing.T) {
	// Test that all trigger types implement the Trigger interface
	var _ Trigger = &FixedTimeTrigger{}
	var _ Trigger = &SunTrigger{}
	var _ Trigger = &CompositeDailySchedule{}
}
