package core

import (
	"context"
	"testing"
	"time"

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

// Integrations put arbitrary shapes in an event's data. A call_service event
// carries entity_id as a list, which does not fit the state_changed schema;
// decoding both together dropped the whole event.
func TestParseEventKeepsEventsWhoseDataCollidesWithStateChanged(t *testing.T) {
	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"call_service","data":{
		"domain":"light","service":"turn_on",
		"service_data":{"entity_id":["light.a","light.b"]},
		"entity_id":["light.a","light.b"]}}}`))

	assert.Equal(t, "call_service", ev.Type, "the event type survives a payload we cannot model")
}

func TestParseEventMarksEntityCreation(t *testing.T) {
	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.new",
		"old_state":null,
		"new_state":{"entity_id":"light.new","state":"on"}}}}`))

	assert.True(t, ev.Created)
	assert.False(t, ev.Deleted)
	assert.Equal(t, "on", ev.To.State)
	assert.Empty(t, ev.From.State)
}

func TestParseEventMarksEntityDeletion(t *testing.T) {
	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.gone",
		"old_state":{"entity_id":"light.gone","state":"on"},
		"new_state":null}}}`))

	assert.True(t, ev.Deleted)
	assert.False(t, ev.Created)
	assert.Equal(t, "on", ev.From.State)
}

// A deleted entity did not transition to anything, so an automation watching
// it must not fire with an empty state that reads like a real one.
func TestStateChangedIgnoresDeletion(t *testing.T) {
	trig := StateChanged("light.gone")

	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.gone",
		"old_state":{"entity_id":"light.gone","state":"on"},
		"new_state":null}}}`))

	assert.False(t, trig.Matches(ev))
}

func TestStateChangedFiresOnCreation(t *testing.T) {
	trig := StateChanged("light.new").To("on")

	ev := parseEvent([]byte(`{"type":"event","event":{"event_type":"state_changed","data":{
		"entity_id":"light.new",
		"old_state":null,
		"new_state":{"entity_id":"light.new","state":"on"}}}}`))

	assert.True(t, trig.Matches(ev), "an entity appearing in the state it is watched for is a real transition")
}

// Nothing is before midnight. Left to the start/end comparison, an empty
// window reads as a range wrapping the whole day, which is its exact opposite.
func TestBeforeMidnightNeverHolds(t *testing.T) {
	c := BeforeTime(TimeOfDay(0, 0))

	for _, hour := range []int{0, 6, 12, 23} {
		assert.False(t, evalAt(t, c, hour, 0), "at %02d:00", hour)
	}
}

func TestAfterMidnightAlwaysHolds(t *testing.T) {
	c := AfterTime(TimeOfDay(0, 0))

	for _, hour := range []int{0, 6, 12, 23} {
		assert.True(t, evalAt(t, c, hour, 0), "at %02d:00", hour)
	}
}

func TestEmptyDateConditionsFailTheBuild(t *testing.T) {
	for name, cond := range map[string]Condition{
		"OnDates":    OnDates(),
		"OnWeekdays": OnWeekdays(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewAutomation("a").On(Daily(TimeOfDay(9, 0))).When(cond).Do(noAction).Build()
			assert.ErrorIs(t, err, ErrInvalidArgs,
				"a condition that can never hold is a mistake, not a configuration")
		})
	}
}

// bothFamilies implements the schedule and event contracts at once. A type
// switch would silently pick one and drop the other half.
type bothFamilies struct{}

func (bothFamilies) trigger()                             {}
func (bothFamilies) NextTime(time.Time) (time.Time, bool) { return time.Time{}, false }
func (bothFamilies) Subscriptions() []Subscription        { return nil }
func (bothFamilies) Matches(Event) bool                   { return false }

func TestRegisterRejectsATriggerInBothFamilies(t *testing.T) {
	app := testApp()

	a := NewAutomation("ambiguous").On(bothFamilies{}).Do(noAction).MustBuild()
	err := app.RegisterAutomations(a)

	assert.ErrorIs(t, err, ErrInvalidAutomation)
	assert.Contains(t, err.Error(), "both trigger families")
}

func TestFailedSeedClosesItsWindow(t *testing.T) {
	c := newEntityCache()

	c.beginSeed()
	c.apply(entity("light.kitchen", "on"))
	c.abandonSeed()

	// With the window still open the touched set would keep growing against a
	// snapshot that is never coming.
	c.beginSeed()
	c.finishSeed([]EntityState{entity("light.kitchen", "off")})

	got, _ := c.get("light.kitchen")
	assert.Equal(t, "off", got.State, "the abandoned window must not protect stale state")
}
