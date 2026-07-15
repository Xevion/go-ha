package ha

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/internal"
)

// stateWith builds a seeded state, which serves reads from the cache and never
// touches HTTP. Conditions run against the real reader rather than a stand-in.
func stateWith(entities ...EntityState) *state {
	s := &state{cache: newEntityCache()}
	s.cache.beginSeed()
	s.cache.finishSeed(entities)
	return s
}

func evalAgainst(t *testing.T, c Condition, s *state) (bool, error) {
	t.Helper()
	return c.Eval(context.Background(), EvalContext{Clock: testClock(), State: s})
}

func TestStateIs(t *testing.T) {
	s := stateWith(entity("light.kitchen", "on"))

	got, err := evalAgainst(t, StateIs("light.kitchen", "on"), s)
	require.NoError(t, err)
	assert.True(t, got)

	got, err = evalAgainst(t, StateIs("light.kitchen", "off"), s)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestStateIsNot(t *testing.T) {
	s := stateWith(entity("light.kitchen", "on"))

	got, err := evalAgainst(t, StateIsNot("light.kitchen", "off"), s)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestStateIsOneOf(t *testing.T) {
	s := stateWith(entity("media_player.tv", "playing"))

	got, err := evalAgainst(t, StateIsOneOf("media_player.tv", "playing", "paused"), s)
	require.NoError(t, err)
	assert.True(t, got)

	got, err = evalAgainst(t, StateIsOneOf("media_player.tv", "off", "idle"), s)
	require.NoError(t, err)
	assert.False(t, got)
}

// An entity the reader cannot resolve leaves the condition undecided, so the
// automation's error policy gets to choose rather than the condition assuming.
func TestStateIsIsUndecidedForAnUnknownEntity(t *testing.T) {
	s := stateWith(entity("light.kitchen", "on"))

	_, err := evalAgainst(t, StateIs("light.missing", "on"), s)
	assert.ErrorIs(t, err, internal.ErrEntityNotFound)
}

func TestStateIsNotIsUndecidedForAnUnknownEntity(t *testing.T) {
	s := stateWith(entity("light.kitchen", "on"))

	// Negation must not turn "cannot read it" into "it is not on".
	_, err := evalAgainst(t, StateIsNot("light.missing", "on"), s)
	assert.ErrorIs(t, err, internal.ErrEntityNotFound)
}
