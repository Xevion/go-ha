package hatest_test

import (
	"context"
	"testing"
	"time"

	ha "github.com/Xevion/go-ha"
	"github.com/Xevion/go-ha/hatest"
	"github.com/Xevion/go-ha/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server is meant to stand behind a real App, so the surface that matters
// is what an App sees: a seeded world, state changes and events that fire
// automations, and the service calls those automations make.
func TestServerDrivesARealApp(t *testing.T) {
	s := hatest.New(t)
	s.SetState("binary_sensor.motion", "off")
	s.SetState("light.hall", "off")
	s.SetSun(true, time.Now().Add(6*time.Hour), time.Now().Add(8*time.Hour))

	app, err := ha.NewApp(types.NewAppRequest{URL: s.URL(), HAAuthToken: hatest.Token})
	require.NoError(t, err)
	defer app.Close()

	require.NoError(t, app.RegisterAutomations(
		ha.NewAutomation("motion light").
			On(ha.StateChanged("binary_sensor.motion").To("on")).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.Light.TurnOn("light.hall")
			}).
			MustBuild(),
		ha.NewAutomation("on custom event").
			On(ha.EventFired("custom_event")).
			Do(func(_ context.Context, run ha.Run) error {
				return run.Services.HomeAssistant.TurnOn("light.hall")
			}).
			MustBuild(),
	))

	go func() { _ = app.Start() }()
	time.Sleep(100 * time.Millisecond) // let the connection, subscriptions and seed settle

	// Seeded state is served over REST and read back from the cache.
	sun, err := app.State().Get("sun.sun")
	require.NoError(t, err)
	assert.Equal(t, "above_horizon", sun.State)

	s.ChangeState("binary_sensor.motion", "on")
	s.Fire("custom_event", map[string]any{"foo": "bar"})

	calls := s.WaitForCalls(2)
	require.Len(t, calls, 2)

	// One call per automation; the order the two arrive in is not guaranteed.
	got := map[string]string{}
	for _, c := range calls {
		got[c.Domain] = c.Service
	}
	assert.Equal(t, "turn_on", got["light"], "the state change turned the light on")
	assert.Equal(t, "turn_on", got["homeassistant"], "the fired event turned the light on")

	// Calls reports the same history WaitForCalls returned.
	assert.Len(t, s.Calls(), 2)

	// Removing an entity is announced without disturbing the running app.
	s.RemoveState("light.hall")
}
