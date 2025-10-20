package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var clockBase = time.Date(2025, time.October, 19, 22, 31, 56, 0, time.UTC)

func TestFakeClockNow(t *testing.T) {
	c := NewFakeClock(clockBase)

	assert.Equal(t, clockBase, c.Now())
	assert.Equal(t, clockBase, c.Now(), "repeated reads must not drift")
}

func TestFakeClockAdvance(t *testing.T) {
	tests := []struct {
		name     string
		steps    []time.Duration
		expected time.Duration
	}{
		{"single step", []time.Duration{time.Hour}, time.Hour},
		{"accumulates", []time.Duration{time.Hour, 30 * time.Minute}, 90 * time.Minute},
		{"zero is a no-op", []time.Duration{0}, 0},
		{"negative moves back", []time.Duration{time.Hour, -2 * time.Hour}, -time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFakeClock(clockBase)
			for _, step := range tt.steps {
				c.Advance(step)
			}
			assert.Equal(t, clockBase.Add(tt.expected), c.Now())
		})
	}
}

func TestFakeClockSet(t *testing.T) {
	c := NewFakeClock(clockBase)
	c.Advance(6 * time.Hour)

	target := time.Date(2026, time.March, 2, 9, 15, 3, 0, time.UTC)
	c.Set(target)

	assert.Equal(t, target, c.Now(), "Set replaces the instant, it does not offset it")
}

func TestFakeClockCarbon(t *testing.T) {
	c := NewFakeClock(clockBase)

	assert.Equal(t, c.Now(), c.Carbon().StdTime(), "Carbon and Now must report the same instant")

	c.Advance(time.Hour)
	assert.Equal(t, c.Now(), c.Carbon().StdTime(), "Carbon must follow Advance")
}

func TestRealClockCarbonTracksNow(t *testing.T) {
	c := RealClock{}

	before := c.Now()
	got := c.Carbon().StdTime()
	after := c.Now()

	assert.False(t, got.Before(before), "Carbon must not predate the read before it")
	assert.False(t, got.After(after), "Carbon must not postdate the read after it")
}
