package hatest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClock(t *testing.T) {
	start := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	c := NewClock(start)
	assert.Equal(t, start, c.Now())

	c.Advance(90 * time.Minute)
	assert.Equal(t, start.Add(90*time.Minute), c.Now())

	c.Advance(-30 * time.Minute)
	assert.Equal(t, start.Add(60*time.Minute), c.Now())

	reset := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Set(reset)
	assert.Equal(t, reset, c.Now())
}
