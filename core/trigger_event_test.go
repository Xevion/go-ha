package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stateChange(entityID, from, to string) Event {
	return Event{
		Type:     eventStateChanged,
		EntityID: entityID,
		From:     EntityState{EntityID: entityID, State: from},
		To:       EntityState{EntityID: entityID, State: to},
	}
}

func TestStateChangedMatchesItsEntities(t *testing.T) {
	trig := StateChanged("light.kitchen", "light.hall")

	assert.True(t, trig.Matches(stateChange("light.kitchen", "off", "on")))
	assert.True(t, trig.Matches(stateChange("light.hall", "off", "on")))
	assert.False(t, trig.Matches(stateChange("light.porch", "off", "on")))
}

// Home Assistant emits state_changed for attribute-only updates, where the
// state itself is unchanged. Firing on those surprises everyone.
func TestStateChangedIgnoresUnchangedState(t *testing.T) {
	trig := StateChanged("device_tracker.phone")

	assert.False(t, trig.Matches(stateChange("device_tracker.phone", "home", "home")))
}

func TestStateChangedNarrowsByTransition(t *testing.T) {
	toOn := StateChanged("light.kitchen").To("on")
	assert.True(t, toOn.Matches(stateChange("light.kitchen", "off", "on")))
	assert.False(t, toOn.Matches(stateChange("light.kitchen", "on", "off")))

	fromOff := StateChanged("light.kitchen").From("off")
	assert.True(t, fromOff.Matches(stateChange("light.kitchen", "off", "on")))
	assert.False(t, fromOff.Matches(stateChange("light.kitchen", "on", "off")))
}

// Every builder stage returns a copy, so narrowing one branch must not reach
// back into the chain it came from.
func TestStateChangedBranchesAreIndependent(t *testing.T) {
	base := StateChanged("light.kitchen")
	toOn := base.To("on")
	toOff := base.To("off")

	assert.True(t, toOn.Matches(stateChange("light.kitchen", "off", "on")))
	assert.True(t, toOff.Matches(stateChange("light.kitchen", "on", "off")))
	assert.True(t, base.Matches(stateChange("light.kitchen", "on", "off")),
		"the unnarrowed trigger must still match either direction")
}

func TestStateChangedSubscribesToStateChanged(t *testing.T) {
	subs := StateChanged("light.kitchen").Subscriptions()
	require.Len(t, subs, 1)
	assert.Equal(t, eventStateChanged, subs[0].EventType)
}

func TestEventFiredMatchesItsTypes(t *testing.T) {
	trig := EventFired("call_service", "automation_triggered")

	assert.True(t, trig.Matches(Event{Type: "call_service"}))
	assert.True(t, trig.Matches(Event{Type: "automation_triggered"}))
	assert.False(t, trig.Matches(Event{Type: "state_changed"}))
}

func TestEventFiredSubscribesToEachType(t *testing.T) {
	subs := EventFired("call_service", "zha_event").Subscriptions()
	require.Len(t, subs, 2)
	assert.Equal(t, "call_service", subs[0].EventType)
	assert.Equal(t, "zha_event", subs[1].EventType)
}

func TestEventFiredNeedsAtLeastOneType(t *testing.T) {
	assert.ErrorIs(t, EventFired().validate(), ErrInvalidArgs)
}

// A state_changed trigger must not be fired by some other event type that
// happens to reach it.
func TestStateChangedIgnoresOtherEventTypes(t *testing.T) {
	trig := StateChanged("light.kitchen")
	assert.False(t, trig.Matches(Event{Type: "call_service", EntityID: "light.kitchen"}))
}
