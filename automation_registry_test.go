package ha

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/internal"
)

// testApp builds an App with everything dispatch touches and nothing it does
// not, so registration can be exercised without a Home Assistant.
func testApp(entities ...EntityState) *App {
	clock := testClock()
	return &App{
		ctx:         context.Background(),
		clock:       clock,
		state:       stateWith(entities...),
		schedules:   newScheduler(clock),
		intervals:   newScheduler(clock),
		automations: map[string][]binding{},
		runners:     map[*runner]struct{}{},
	}
}

func stateChangedJSON(entityID, from, to string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"type": "event",
		"event": map[string]any{
			"event_type": eventStateChanged,
			"data": map[string]any{
				"entity_id": entityID,
				"old_state": map[string]any{"entity_id": entityID, "state": from},
				"new_state": map[string]any{"entity_id": entityID, "state": to},
			},
		},
	})
	return raw
}

func TestRegisterRejectsAnUnbuiltAutomation(t *testing.T) {
	app := testApp()

	// Zero value rather than one from Build, so it has no runner.
	err := app.RegisterAutomations(Automation{name: "raw"})
	assert.ErrorIs(t, err, ErrInvalidAutomation)
}

func TestRegisterQueuesScheduleTriggers(t *testing.T) {
	app := testApp()

	require.NoError(t, app.RegisterAutomations(
		NewAutomation("nightly").On(Daily(TimeOfDay(23, 0))).Do(noAction).MustBuild(),
	))

	assert.Equal(t, 1, app.schedules.len())
}

func TestRegisterRoutesEventTriggers(t *testing.T) {
	app := testApp()

	require.NoError(t, app.RegisterAutomations(
		NewAutomation("motion").On(StateChanged("binary_sensor.motion")).Do(noAction).MustBuild(),
	))

	assert.Len(t, app.automations[eventStateChanged], 1)
}

// One automation may hold both families, which is the point of the marker
// interface: "at sunset, or when the door opens" is a single rule.
func TestRegisterHandlesMixedTriggerFamilies(t *testing.T) {
	app := testApp()

	require.NoError(t, app.RegisterAutomations(
		NewAutomation("mixed").
			On(Daily(TimeOfDay(20, 0)), StateChanged("binary_sensor.door").To("on")).
			Do(noAction).
			MustBuild(),
	))

	assert.Equal(t, 1, app.schedules.len(), "the schedule half is queued")
	assert.Len(t, app.automations[eventStateChanged], 1, "the event half is routed")
}

func TestDispatchRunsAMatchingAutomation(t *testing.T) {
	app := testApp(entity("binary_sensor.motion", "off"))

	fired := make(chan string, 1)
	a := NewAutomation("motion").
		On(StateChanged("binary_sensor.motion").To("on")).
		Do(func(_ context.Context, run Run) error {
			fired <- run.Event.EntityID
			return nil
		}).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "off", "on"))

	a.runtime.wait()

	require.Len(t, fired, 1)
	assert.Equal(t, "binary_sensor.motion", <-fired)
}

func TestDispatchSkipsANonMatchingTrigger(t *testing.T) {
	app := testApp()

	a := NewAutomation("motion").
		On(StateChanged("binary_sensor.motion").To("on")).
		Do(func(context.Context, Run) error { t.Error("must not run"); return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "on", "off"))
	a.runtime.wait()
}

func TestDispatchAppliesConditions(t *testing.T) {
	app := testApp(
		entity("binary_sensor.motion", "off"),
		entity("light.kitchen", "on"),
	)

	a := NewAutomation("motion").
		On(StateChanged("binary_sensor.motion").To("on")).
		When(StateIs("light.kitchen", "off")).
		Do(func(context.Context, Run) error { t.Error("condition does not hold"); return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "off", "on"))
	a.runtime.wait()
}

// The runner keys on the triggering entity, so one automation watching several
// entities holds a throttle window for each rather than one between them.
func TestDispatchThrottlesEachEntitySeparately(t *testing.T) {
	app := testApp()

	fired := make(chan string, 4)
	a := NewAutomation("many").
		On(StateChanged("sensor.a", "sensor.b")).
		Throttle(time.Hour).
		Mode(ModeParallel).
		Do(func(_ context.Context, run Run) error {
			fired <- run.Event.EntityID
			return nil
		}).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("sensor.a", "0", "1"))
	app.dispatchEvent(stateChangedJSON("sensor.a", "1", "2"))
	app.dispatchEvent(stateChangedJSON("sensor.b", "0", "1"))
	a.runtime.wait()

	close(fired)
	var got []string
	for e := range fired {
		got = append(got, e)
	}
	assert.ElementsMatch(t, []string{"sensor.a", "sensor.b"}, got,
		"the second sensor.a event is throttled, sensor.b is not")
}

func TestScheduledAutomationFiresThroughTheScheduler(t *testing.T) {
	clock := internal.NewFakeClock(time.Date(2026, 7, 19, 8, 0, 0, 0, time.Local))
	app := testApp()
	app.clock = clock
	app.schedules = newScheduler(clock)

	fired := make(chan struct{}, 1)
	a := NewAutomation("morning").
		On(Daily(TimeOfDay(9, 0))).
		Do(func(context.Context, Run) error { fired <- struct{}{}; return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	clock.Advance(2 * time.Hour)
	assert.Equal(t, 1, app.schedules.runDue(clock.Now()))

	a.runtime.wait()
	assert.Len(t, fired, 1)
}
