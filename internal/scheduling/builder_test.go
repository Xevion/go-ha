package scheduling

import (
	"fmt"
	"testing"
	"time"

	"github.com/Xevion/go-ha/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchedule(t *testing.T) {
	builder := NewSchedule()
	assert.NotNil(t, builder)
	assert.Empty(t, builder.errors)
	assert.Empty(t, builder.triggers)
	assert.NotNil(t, builder.hashes)
}

func TestDailyScheduleBuilder_OnFixedTime(t *testing.T) {
	tests := []struct {
		name        string
		hour        int
		minute      int
		expectError bool
	}{
		{
			name:        "valid time",
			hour:        12,
			minute:      30,
			expectError: false,
		},
		{
			name:        "midnight",
			hour:        0,
			minute:      0,
			expectError: false,
		},
		{
			name:        "invalid hour negative",
			hour:        -1,
			minute:      30,
			expectError: true,
		},
		{
			name:        "invalid hour too high",
			hour:        24,
			minute:      30,
			expectError: true,
		},
		{
			name:        "invalid minute negative",
			hour:        12,
			minute:      -1,
			expectError: true,
		},
		{
			name:        "invalid minute too high",
			hour:        12,
			minute:      60,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewSchedule()
			result := builder.OnFixedTime(tt.hour, tt.minute)

			assert.Equal(t, builder, result) // Should return self for chaining

			if tt.expectError {
				assert.Len(t, builder.errors, 1)
			} else {
				assert.Empty(t, builder.errors)
				assert.Len(t, builder.triggers, 1)
			}
		})
	}
}

func TestDailyScheduleBuilder_OnSunrise(t *testing.T) {
	tests := []struct {
		name        string
		offset      []types.DurationString
		expectError bool
	}{
		{
			name:        "with offset",
			offset:      []types.DurationString{"30m"},
			expectError: false,
		},
		{
			name:        "with negative offset",
			offset:      []types.DurationString{"-1h"},
			expectError: false,
		},
		{
			name:        "no offset",
			offset:      []types.DurationString{},
			expectError: true,
		},
		{
			name:        "invalid duration",
			offset:      []types.DurationString{"invalid"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewSchedule()
			result := builder.OnSunrise(tt.offset...)

			assert.Equal(t, builder, result) // Should return self for chaining

			if tt.expectError {
				assert.Len(t, builder.errors, 1)
			} else {
				assert.Empty(t, builder.errors)
				assert.Len(t, builder.triggers, 1)
			}
		})
	}
}

func TestDailyScheduleBuilder_OnSunset(t *testing.T) {
	tests := []struct {
		name        string
		offset      []types.DurationString
		expectError bool
	}{
		{
			name:        "with offset",
			offset:      []types.DurationString{"1h"},
			expectError: false,
		},
		{
			name:        "with negative offset",
			offset:      []types.DurationString{"-30m"},
			expectError: false,
		},
		{
			name:        "no offset",
			offset:      []types.DurationString{},
			expectError: true,
		},
		{
			name:        "invalid duration",
			offset:      []types.DurationString{"invalid"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewSchedule()
			result := builder.OnSunset(tt.offset...)

			assert.Equal(t, builder, result) // Should return self for chaining

			if tt.expectError {
				assert.Len(t, builder.errors, 1)
			} else {
				assert.Empty(t, builder.errors)
				assert.Len(t, builder.triggers, 1)
			}
		})
	}
}

func TestDailyScheduleBuilder_DuplicateTriggers(t *testing.T) {
	builder := NewSchedule()

	// Add the same fixed time trigger twice
	builder.OnFixedTime(12, 30)
	builder.OnFixedTime(12, 30)

	assert.Len(t, builder.errors, 1)
	assert.Len(t, builder.triggers, 1) // Only one should be added
	assert.Contains(t, builder.errors[0].Error(), "duplicate trigger")
}

func TestDailyScheduleBuilder_Build_Success(t *testing.T) {
	tests := []struct {
		name          string
		setupBuilder  func(*DailyScheduleBuilder)
		expectedType  string
		expectedCount int
	}{
		{
			name: "single fixed time trigger",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnFixedTime(12, 30)
			},
			expectedType:  "*scheduling.FixedTimeTrigger",
			expectedCount: 1,
		},
		{
			name: "single sunrise trigger",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnSunrise("30m")
			},
			expectedType:  "*scheduling.SunTrigger",
			expectedCount: 1,
		},
		{
			name: "multiple triggers",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnFixedTime(8, 0)
				b.OnFixedTime(12, 0)
				b.OnSunrise("1h")
			},
			expectedType:  "*scheduling.CompositeDailySchedule",
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewSchedule()
			tt.setupBuilder(builder)

			trigger, err := builder.Build()

			require.NoError(t, err)
			require.NotNil(t, trigger)
			assert.Equal(t, tt.expectedType, fmt.Sprintf("%T", trigger))

			// Test that the trigger works
			now := time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local)
			result := trigger.NextTime(now)
			assert.NotNil(t, result)
		})
	}
}

func TestDailyScheduleBuilder_Build_Errors(t *testing.T) {
	tests := []struct {
		name         string
		setupBuilder func(*DailyScheduleBuilder)
		expectError  bool
	}{
		{
			name: "no triggers",
			setupBuilder: func(b *DailyScheduleBuilder) {
				// Don't add any triggers
			},
			expectError: true,
		},
		{
			name: "invalid hour",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnFixedTime(25, 0) // Invalid hour
			},
			expectError: true,
		},
		{
			name: "invalid minute",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnFixedTime(12, 60) // Invalid minute
			},
			expectError: true,
		},
		{
			name: "no offset for sun trigger",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnSunrise() // No offset
			},
			expectError: true,
		},
		{
			name: "invalid duration",
			setupBuilder: func(b *DailyScheduleBuilder) {
				b.OnSunset("invalid") // Invalid duration
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewSchedule()
			tt.setupBuilder(builder)

			trigger, err := builder.Build()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, trigger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, trigger)
			}
		})
	}
}

func TestDailyScheduleBuilder_Chaining(t *testing.T) {
	builder := NewSchedule()

	// Test method chaining
	result := builder.
		OnFixedTime(8, 0).
		OnFixedTime(12, 0).
		OnSunrise("30m")

	assert.Equal(t, builder, result)
	assert.Len(t, builder.triggers, 3)
	assert.Empty(t, builder.errors)
}

func TestDailyScheduleBuilder_NextTime_Integration(t *testing.T) {
	builder := NewSchedule()
	builder.OnFixedTime(8, 0).
		OnFixedTime(12, 0).
		OnFixedTime(18, 0)

	trigger, err := builder.Build()
	require.NoError(t, err)

	// Test at different times
	tests := []struct {
		name     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "before all triggers",
			now:      time.Date(2025, 8, 2, 6, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 2, 8, 0, 0, 0, time.Local),
		},
		{
			name:     "between triggers",
			now:      time.Date(2025, 8, 2, 10, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 2, 12, 0, 0, 0, time.Local),
		},
		{
			name:     "after all triggers",
			now:      time.Date(2025, 8, 2, 20, 0, 0, 0, time.Local),
			expected: time.Date(2025, 8, 3, 8, 0, 0, 0, time.Local),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trigger.NextTime(tt.now)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, *result)
		})
	}
}
