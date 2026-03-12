package ha

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityListenerBranchesGetTheirOwnRuntime(t *testing.T) {
	base := NewEntityListener().
		EntityIds("light.kitchen").
		Call(func(*Service, StateReader, EntityData) {})

	slow := base.Throttle("5m").Build()
	fast := base.Build()

	require.NotNil(t, slow.runtime)
	require.NotNil(t, fast.runtime)
	assert.NotSame(t, slow.runtime, fast.runtime,
		"two listeners built from one chain must not share a throttle window")
}

func TestEventListenerBranchesGetTheirOwnRuntime(t *testing.T) {
	base := NewEventListener().
		EventTypes("call_service").
		Call(func(*Service, StateReader, EventData) {})

	slow := base.Throttle("5m").Build()
	fast := base.Build()

	require.NotNil(t, slow.runtime)
	require.NotNil(t, fast.runtime)
	assert.NotSame(t, slow.runtime, fast.runtime)
}

func TestUnthrottledSiblingDoesNotConsumeAThrottledWindow(t *testing.T) {
	clock := testClock()

	base := NewEntityListener().
		EntityIds("sensor.power").
		Call(func(*Service, StateReader, EntityData) {})

	slow := base.Throttle("5m").Build()
	fast := base.Build()

	// The unthrottled listener fires freely and stamps its own window. Sharing
	// one runtime would let it restamp the throttled listener's, which then
	// never fires at all while the entity stays busy.
	for range 3 {
		assert.True(t, fast.runtime.claim(clock, fast.throttle))
	}

	assert.True(t, slow.runtime.claim(clock, slow.throttle),
		"a sibling's runs must not consume this listener's throttle window")
}

func TestSiblingDelayTimersAreIndependent(t *testing.T) {
	base := NewEntityListener().
		EntityIds("binary_sensor.door").
		Call(func(*Service, StateReader, EntityData) {})

	first := base.Build()
	second := base.Build()

	fired := make(chan struct{}, 1)
	first.runtime.arm(time.AfterFunc(time.Millisecond, func() { fired <- struct{}{} }))

	// Arming the sibling must not cancel this one's pending callback.
	second.runtime.arm(time.AfterFunc(time.Hour, func() {}))

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("a sibling's delay timer cancelled this listener's pending callback")
	}

	second.runtime.disarm()
}
