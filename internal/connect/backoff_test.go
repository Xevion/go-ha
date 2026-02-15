package connect

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedRand returns a generator seeded identically every time, so jittered
// delays are reproducible across runs.
func fixedRand() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}

func TestBackoffRawDoublesThenSaturates(t *testing.T) {
	b := newBackoff(fixedRand())

	assert.Equal(t, time.Second, b.raw(0))
	assert.Equal(t, 2*time.Second, b.raw(1))
	assert.Equal(t, 4*time.Second, b.raw(2))
	assert.Equal(t, 32*time.Second, b.raw(5))
	assert.Equal(t, defaultBackoffMax, b.raw(6), "6 doublings passes the cap")
	assert.Equal(t, defaultBackoffMax, b.raw(500), "a shift this wide must clamp, not overflow")
}

func TestBackoffNextStaysWithinJitterBand(t *testing.T) {
	b := newBackoff(fixedRand())

	for attempt := range 12 {
		raw := b.raw(attempt)
		got := b.next()

		low := time.Duration(float64(raw) * (1 - defaultBackoffJitter))
		high := time.Duration(float64(raw) * (1 + defaultBackoffJitter))
		assert.GreaterOrEqual(t, got, low, "attempt %d below the jitter band", attempt)
		assert.LessOrEqual(t, got, high, "attempt %d above the jitter band", attempt)
	}
}

func TestBackoffNextIsNeverNegative(t *testing.T) {
	b := newBackoff(fixedRand())
	b.jitter = 5 // absurd, but a negative sleep must never escape

	for range 50 {
		assert.GreaterOrEqual(t, b.next(), time.Duration(0))
	}
}

func TestBackoffJitterActuallyVaries(t *testing.T) {
	b := newBackoff(fixedRand())
	b.max = time.Hour // keep every attempt below the cap, which erases variation

	seen := map[time.Duration]bool{}
	for range 10 {
		seen[b.next()] = true
	}
	assert.Greater(t, len(seen), 1, "identical delays mean jitter is not being applied")
}

func TestBackoffReset(t *testing.T) {
	b := newBackoff(fixedRand())
	for range 5 {
		b.next()
	}
	require.NotZero(t, b.attempt)

	b.reset()
	assert.Zero(t, b.attempt)
	assert.LessOrEqual(t, b.next(), time.Duration(float64(time.Second)*(1+defaultBackoffJitter)),
		"after a reset the sequence must start from the base delay again")
}

func TestBackoffZeroJitterIsExact(t *testing.T) {
	b := newBackoff(fixedRand())
	b.jitter = 0

	assert.Equal(t, time.Second, b.next())
	assert.Equal(t, 2*time.Second, b.next())
	assert.Equal(t, 4*time.Second, b.next())
}
