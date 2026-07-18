package ha_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ha "github.com/Xevion/go-ha"
	"github.com/Xevion/go-ha/hatest"
	"github.com/Xevion/go-ha/types"
)

// newApp connects a real App to an in-process Home Assistant.
func newApp(t *testing.T, server *hatest.Server) *ha.App {
	t.Helper()

	app, err := ha.NewApp(types.NewAppRequest{
		URL:         server.URL(),
		HAAuthToken: hatest.Token,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })

	return app
}

// start runs the app and waits for it to be live, since automations are gated
// until it is.
func start(t *testing.T, app *ha.App) {
	t.Helper()

	go func() { _ = app.Start() }()
	t.Cleanup(func() { _ = app.Close() })
	time.Sleep(100 * time.Millisecond)
}

func TestAutomationRunsAgainstAFakeHomeAssistant(t *testing.T) {
	server := hatest.New(t)
	server.SetState("binary_sensor.motion", "off")
	server.SetState("light.hall", "off")

	app := newApp(t, server)
	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("hall light").
			On(ha.StateChanged("binary_sensor.motion").To("on")).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	))
	start(t, app)

	server.ChangeState("binary_sensor.motion", "on")

	calls := server.WaitForCalls(1)
	assert.Equal(t, "light", calls[0].Domain)
	assert.Equal(t, "turn_on", calls[0].Service)
	assert.Equal(t, "light.hall", calls[0].EntityID)
}

// The condition reads through the cache, which the app seeded on connect and
// keeps current from the event stream.
func TestConditionsReadSeededState(t *testing.T) {
	server := hatest.New(t)
	server.SetState("binary_sensor.motion", "off")
	server.SetState("light.hall", "on")

	app := newApp(t, server)
	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("hall light").
			On(ha.StateChanged("binary_sensor.motion").To("on")).
			When(ha.StateIs("light.hall", "off")).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	))
	start(t, app)

	server.ChangeState("binary_sensor.motion", "on")
	time.Sleep(200 * time.Millisecond)
	assert.Empty(t, server.Calls(), "the light is already on, so the condition does not hold")

	// Turning it off makes the condition hold, and the cache learns that from
	// the event rather than from a fresh request.
	server.ChangeState("light.hall", "off")
	server.ChangeState("binary_sensor.motion", "off")
	server.ChangeState("binary_sensor.motion", "on")

	server.WaitForCalls(1)
}

func TestSunTriggerReadsTheServersTimes(t *testing.T) {
	server := hatest.New(t)
	server.SetSun(true, time.Now().Add(12*time.Hour), time.Now().Add(300*time.Millisecond))
	server.SetState("light.porch", "off")

	app := newApp(t, server)
	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("porch light").
			On(ha.Sunset()).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.porch")
			}).
			MustBuild(),
	))
	start(t, app)

	calls := server.WaitForCalls(1)
	assert.Equal(t, "light.porch", calls[0].EntityID)
}

func TestThrottleHoldsAcrossRealEvents(t *testing.T) {
	server := hatest.New(t)
	server.SetState("binary_sensor.motion", "off")

	app := newApp(t, server)
	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("noisy").
			On(ha.StateChanged("binary_sensor.motion").To("on")).
			Throttle(time.Hour).
			Mode(ha.ModeParallel).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	))
	start(t, app)

	for range 3 {
		server.ChangeState("binary_sensor.motion", "on")
		server.ChangeState("binary_sensor.motion", "off")
	}

	server.WaitForCalls(1)
	time.Sleep(200 * time.Millisecond)
	assert.Len(t, server.Calls(), 1, "the throttle window covers the rest")
}

func TestEventTriggerFiresOnACustomEvent(t *testing.T) {
	server := hatest.New(t)

	app := newApp(t, server)
	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("doorbell").
			On(ha.EventFired("zha_event")).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
	))
	start(t, app)

	server.Fire("zha_event", map[string]any{"command": "button_press"})
	server.WaitForCalls(1)
}

func TestAppRefusesABadToken(t *testing.T) {
	server := hatest.New(t)

	_, err := ha.NewApp(types.NewAppRequest{
		URL:         server.URL(),
		HAAuthToken: "not-the-token",
	})
	assert.Error(t, err, "a refused token must surface rather than retry silently")
}
