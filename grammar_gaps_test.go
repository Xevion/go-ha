package ha

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two When calls compose rather than replacing, so conditions can be added by
// separate helpers building on a shared prefix.
func TestWhenAccumulatesConditions(t *testing.T) {
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(yes()).
		When(no()).
		Do(noAction).
		MustBuild()

	ok, err := a.condition.Eval(context.Background(), EvalContext{Clock: testClock()})
	require.NoError(t, err)
	assert.False(t, ok, "both conditions must hold, and the second does not")
}

func TestLimitReachesThePolicy(t *testing.T) {
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		Mode(ModeParallel).
		Limit(3).
		Do(noAction).
		MustBuild()

	assert.Equal(t, 3, a.policy.Limit)
	assert.Equal(t, 3, a.policy.limit())
}

func TestPolicyLimitFallsBackToTheDefault(t *testing.T) {
	assert.Equal(t, defaultLimit, Policy{}.limit())
}

func TestParseEventHandlesNonStateChangedEvents(t *testing.T) {
	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"call_service",
		"data":{"domain":"light","service":"turn_on"}}}`))

	assert.Equal(t, "call_service", ev.Type)
	assert.Empty(t, ev.EntityID, "only state_changed carries an entity")
	assert.NotEmpty(t, ev.Raw, "the payload stays available for types we do not model")
}

func TestParseEventSurvivesMalformedPayloads(t *testing.T) {
	ev := parseEvent([]byte(`not json at all`))
	assert.Empty(t, ev.Type)
	assert.NotEmpty(t, ev.Raw)
}

// An entity removed while a snapshot is in flight must stay removed: the
// removal is newer than the snapshot that is about to land.
func TestRemoveDuringAPendingSeedSticks(t *testing.T) {
	c := newEntityCache()
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	c.beginSeed()
	c.remove("light.kitchen")
	c.finishSeed([]EntityState{entity("light.kitchen", "on")})

	_, ok := c.get("light.kitchen")
	assert.False(t, ok, "a removal that raced the snapshot must not be undone by it")
}

func TestModeNames(t *testing.T) {
	assert.Equal(t, "single", ModeSingle.String())
	assert.Equal(t, "restart", ModeRestart.String())
	assert.Equal(t, "queued", ModeQueued.String())
	assert.Equal(t, "parallel", ModeParallel.String())
}
