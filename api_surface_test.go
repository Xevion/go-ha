package ha_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ha "github.com/Xevion/go-ha"
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
