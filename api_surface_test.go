package ha_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ha "github.com/Xevion/go-ha"
	"github.com/Xevion/go-ha/hatest"
	"github.com/Xevion/go-ha/services"
	"github.com/Xevion/go-ha/types"
)

// fixedClock is what a user outside this module would write. It exists to prove
// they can: EvalContext previously took an internal interface, so a custom
// condition compiled but could never be given a clock to test against.
type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// businessHours is a user-defined condition built only from exported API.
type businessHours struct{}

func (businessHours) Eval(_ context.Context, ec ha.EvalContext) (bool, error) {
	h := ec.Clock.Now().Hour()
	return h >= 9 && h < 17, nil
}

func TestUserDefinedConditionIsTestableFromOutside(t *testing.T) {
	ec := ha.EvalContext{Clock: fixedClock{at: time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)}}

	got, err := businessHours{}.Eval(context.Background(), ec)
	require.NoError(t, err)
	assert.True(t, got)

	ec.Clock = fixedClock{at: time.Date(2026, 7, 19, 20, 0, 0, 0, time.UTC)}
	got, err = businessHours{}.Eval(context.Background(), ec)
	require.NoError(t, err)
	assert.False(t, got)
}

// The same condition has to compose with the built-in combinators, or being
// able to write one is of limited use.
func TestUserDefinedConditionComposes(t *testing.T) {
	c := ha.All(businessHours{}, ha.Not(ha.OnWeekdays(time.Saturday, time.Sunday)))

	// 2026-07-20 is a Monday.
	ec := ha.EvalContext{Clock: fixedClock{at: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}}
	got, err := c.Eval(context.Background(), ec)
	require.NoError(t, err)
	assert.True(t, got)
}

// An automation built entirely from exported API, as a user would write it.
func TestAutomationBuildsFromExportedAPIOnly(t *testing.T) {
	a, err := ha.NewAutomation("porch light").
		On(ha.StateChanged("binary_sensor.motion").To("on"), ha.Daily(ha.TimeOfDay(22, 0))).
		When(businessHours{}, ha.StateIsNot("light.porch", "on")).
		Mode(ha.ModeRestart).
		Throttle(30 * time.Second).
		Do(func(context.Context, ha.Run) error { return nil }).
		Build()

	require.NoError(t, err)
	assert.Equal(t, "porch light", a.Name())
}

// The point of promoting the services package: a user can name a service type
// and write helpers over it. While it lived under internal/ the call worked but
// the type could not be spelled, so this function was impossible to declare.
func dimTo(light *services.Light, entityID services.LightID, brightness int) error {
	return light.TurnOn(entityID, map[string]any{"brightness": brightness})
}

func TestServiceTypesAreNameableFromOutside(t *testing.T) {
	var sent []types.Request
	light := services.BuildService[services.Light](senderFunc(func(r types.Request) error {
		sent = append(sent, r)
		return nil
	}))

	require.NoError(t, dimTo(light, "light.kitchen", 128))
	require.Len(t, sent, 1)

	req, ok := sent[0].(*services.BaseServiceRequest)
	require.True(t, ok)
	assert.Equal(t, "light", req.Domain)
	assert.Equal(t, "turn_on", req.Service)
	assert.Equal(t, "light.kitchen", req.Target.EntityId)
	assert.Equal(t, 128, req.ServiceData["brightness"])
}

type senderFunc func(types.Request) error

func (f senderFunc) Send(r types.Request) error { return f(r) }

// nightly is the shared prefix pattern the docs describe. It only compiles
// because AutomationBuilder is exported: a constructor returning an unexported
// type cannot be named in a signature like this.
func nightly(name string) ha.AutomationBuilder {
	return ha.NewAutomation(name).
		When(ha.SunIsDown()).
		Mode(ha.ModeRestart)
}

func TestSharedBuilderPrefixIsExpressible(t *testing.T) {
	porch, err := nightly("porch").
		On(ha.StateChanged("binary_sensor.porch").To("on")).
		Do(func(context.Context, ha.Run) error { return nil }).
		Build()
	require.NoError(t, err)

	hall, err := nightly("hall").
		On(ha.StateChanged("binary_sensor.hall").To("on")).
		Do(func(context.Context, ha.Run) error { return nil }).
		Build()
	require.NoError(t, err)

	assert.Equal(t, "porch", porch.Name())
	assert.Equal(t, "hall", hall.Name())
}

// Stands in for what cmd/generate emits: constants typed by domain.
var generatedLights = struct {
	Kitchen services.LightID
	Hall    services.LightID
}{Kitchen: "light.kitchen", Hall: "light.hall"}

// Generated constants have to be usable where entities are named, or a typed
// id is a constant you cannot spend.
func TestGeneratedConstantsWorkWithTriggersAndConditions(t *testing.T) {
	a, err := ha.NewAutomation("kitchen").
		On(ha.StateChanged(generatedLights.Kitchen, generatedLights.Hall).To("on")).
		When(ha.StateIsNot(generatedLights.Hall, "unavailable")).
		Do(func(_ context.Context, run ha.Run) error {
			return run.Services.Light.TurnOn(generatedLights.Kitchen)
		}).
		Build()

	require.NoError(t, err)
	assert.Equal(t, "kitchen", a.Name())
}

// Plain strings still work, so the generic parameter costs nothing at call
// sites that do not use generated constants.
func TestPlainStringsStillWork(t *testing.T) {
	_, err := ha.NewAutomation("plain").
		On(ha.StateChanged("binary_sensor.motion").To("on")).
		When(ha.StateIs("light.hall", "off")).
		Do(func(context.Context, ha.Run) error { return nil }).
		Build()

	require.NoError(t, err)
}

// Errors have to be classifiable from outside, or callers are left matching on
// message text.
func TestErrorsAreClassifiableFromOutside(t *testing.T) {
	server := hatest.New(t)
	server.SetState("light.hall", "on")

	app := newApp(t, server)
	time.Sleep(100 * time.Millisecond)

	_, err := app.State().Get("light.nonexistent")
	assert.ErrorIs(t, err, ha.ErrEntityNotFound)
}
