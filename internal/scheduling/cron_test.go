package scheduling_test

import (
	"testing"
	"time"

	"github.com/Xevion/go-ha/internal/scheduling"
)

func TestCronTrigger(t *testing.T) {
	// Use a fixed time for consistent testing
	baseTime := time.Date(2025, 8, 2, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		cron     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "daily at 9am",
			cron:     "0 9 * * *",
			now:      baseTime,
			expected: time.Date(2025, 8, 3, 9, 0, 0, 0, time.UTC),
		},
		{
			name:     "every 15 minutes",
			cron:     "*/15 * * * *",
			now:      baseTime,
			expected: time.Date(2025, 8, 2, 10, 45, 0, 0, time.UTC),
		},
		{
			name: "weekdays at 8am (Saturday)",
			// Base time is a Saturday, so next run should be Monday
			cron:     "0 8 * * 1-5",
			now:      time.Date(2025, 8, 2, 10, 30, 0, 0, time.UTC),
			expected: time.Date(2025, 8, 4, 8, 0, 0, 0, time.UTC),
		},
		{
			name: "weekdays at 8am (Sunday)",
			// Base time is a Sunday, so next run should be Monday
			cron:     "0 8 * * 1-5",
			now:      time.Date(2025, 8, 3, 10, 30, 0, 0, time.UTC),
			expected: time.Date(2025, 8, 4, 8, 0, 0, 0, time.UTC),
		},

		{
			name:     "monthly on 1st",
			cron:     "0 0 1 * *",
			now:      baseTime,
			expected: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "specific time today",
			cron:     "0 14 * * *",
			now:      baseTime,
			expected: time.Date(2025, 8, 2, 14, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger, err := scheduling.NewCronTrigger(tt.cron)
			if err != nil {
				t.Fatalf("Failed to create cron trigger: %v", err)
			}

			next := trigger.NextTime(tt.now)
			if next == nil {
				t.Fatal("Expected next time, got nil")
			}

			if !next.Equal(tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, *next)
			}
		})
	}
}

func TestCronTriggerInvalid(t *testing.T) {
	tests := []struct {
		name       string
		expression string
	}{
		{
			name:       "bad pattern",
			expression: "invalid",
		},
		{
			name:       "requires 5 fields - too few",
			expression: "4",
		},
		{
			name:       "requires 5 fields - missing field",
			expression: "0 9 * *",
		},
		{
			name:       "too many fields",
			expression: "0 9 * * * *",
		},
		{
			name:       "invalid minute",
			expression: "60 9 * * *",
		},
		{
			name:       "invalid hour",
			expression: "0 25 * * *",
		},
		{
			name:       "invalid day of month",
			expression: "0 9 32 * *",
		},
		{
			name:       "invalid month",
			expression: "0 9 * 13 *",
		},
		{
			name:       "invalid day of week",
			expression: "0 9 * * 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := scheduling.NewCronTrigger(tt.expression)
			if err == nil {
				t.Errorf("Expected error for invalid expression %q", tt.expression)
			}
		})
	}
}
