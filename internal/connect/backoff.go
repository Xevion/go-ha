package connect

import (
	"math"
	"math/rand/v2"
	"time"
)

const (
	defaultBackoffBase = time.Second
	defaultBackoffMax  = 60 * time.Second
	// Spread each delay by up to this fraction in either direction, so a set of
	// clients that all lost the same Home Assistant do not retry in lockstep.
	defaultBackoffJitter = 0.2
)

// backoff yields the delay to wait before each successive reconnect attempt.
// The zero value is not usable; construct it with newBackoff.
type backoff struct {
	base   time.Duration
	max    time.Duration
	jitter float64
	rand   *rand.Rand

	attempt int
}

func newBackoff(rng *rand.Rand) *backoff {
	return &backoff{
		base:   defaultBackoffBase,
		max:    defaultBackoffMax,
		jitter: defaultBackoffJitter,
		rand:   rng,
	}
}

// next returns the delay for the current attempt and advances the sequence.
func (b *backoff) next() time.Duration {
	delay := b.raw(b.attempt)
	b.attempt++

	if b.jitter <= 0 {
		return delay
	}

	// rand.Float64 is [0,1), so shift it to [-1,1) to jitter both directions.
	spread := b.jitter * (2*b.rand.Float64() - 1)
	jittered := time.Duration(float64(delay) * (1 + spread))
	if jittered < 0 {
		return 0
	}
	return jittered
}

// raw is the un-jittered delay for an attempt, saturating at max.
func (b *backoff) raw(attempt int) time.Duration {
	// Shifting past the width of the exponent overflows to nonsense, and every
	// value that far out is clamped to max regardless.
	if attempt >= 63 {
		return b.max
	}
	scaled := float64(b.base) * math.Pow(2, float64(attempt))
	if scaled >= float64(b.max) {
		return b.max
	}
	return time.Duration(scaled)
}

// reset returns the sequence to its first delay, called once a connection has
// stayed up long enough to count as genuinely healthy.
func (b *backoff) reset() {
	b.attempt = 0
}
