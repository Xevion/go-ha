package ha

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noAction(context.Context, Run) error { return nil }

func TestBuildRejectsAnIncompleteAutomation(t *testing.T) {
	tests := []struct {
		name    string
		builder AutomationBuilder
		missing string
	}{
		{"no name", NewAutomation("").On(Daily(TimeOfDay(9, 0))).Do(noAction), "name"},
		{"no trigger", NewAutomation("a").Do(noAction), "trigger"},
		{"no action", NewAutomation("a").On(Daily(TimeOfDay(9, 0))), "action"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.builder.Build()
			require.ErrorIs(t, err, ErrInvalidAutomation)
			assert.Contains(t, err.Error(), tt.missing)
		})
	}
}

// Everything wrong is reported at once, so fixing one problem does not reveal
// the next one build by build.
func TestBuildReportsEveryProblemTogether(t *testing.T) {
	_, err := NewAutomation("").Build()
	require.Error(t, err)

	for _, want := range []string{"name", "trigger", "action"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestBuildRejectsAnInvalidTrigger(t *testing.T) {
	_, err := NewAutomation("a").On(Cron("nonsense")).Do(noAction).Build()
	assert.ErrorIs(t, err, ErrInvalidAutomation)
}

// A bad argument buried inside a composed condition still has to surface, or it
// waits and fails at fire time instead.
func TestBuildRejectsAnInvalidNestedCondition(t *testing.T) {
	_, err := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(Any(StateIs("light.kitchen", "on"), Not(TimeBetween(TimeOfDay(99, 0), TimeOfDay(10, 0))))).
		Do(noAction).
		Build()

	assert.ErrorIs(t, err, ErrInvalidTimeOfDay)
}

func TestBuildAcceptsAValidAutomation(t *testing.T) {
	a, err := NewAutomation("kitchen lights").
		On(StateChanged("binary_sensor.motion").To("on")).
		When(BeforeTime(TimeOfDay(7, 0))).
		Mode(ModeRestart).
		Throttle(time.Minute).
		Do(noAction).
		Build()

	require.NoError(t, err)
	assert.Equal(t, "kitchen lights", a.Name())
	assert.Equal(t, ModeRestart, a.policy.Mode)
	assert.NotNil(t, a.runtime)
}

func TestMustBuildPanicsOnAnInvalidAutomation(t *testing.T) {
	assert.Panics(t, func() { NewAutomation("").MustBuild() })
}

// Every builder stage returns a copy, so a shared prefix must not leak state
// between the automations branched off it.
func TestBuilderBranchesAreIndependent(t *testing.T) {
	base := NewAutomation("base").On(Daily(TimeOfDay(9, 0))).Do(noAction)

	morning, err := base.On(Daily(TimeOfDay(10, 0))).Build()
	require.NoError(t, err)
	evening, err := base.On(Daily(TimeOfDay(20, 0))).Build()
	require.NoError(t, err)

	assert.Len(t, morning.triggers, 2)
	assert.Len(t, evening.triggers, 2, "one branch must not see the other's trigger")
	assert.NotSame(t, morning.runtime, evening.runtime,
		"two automations must not share a throttle window")
}

func TestBranchedAutomationsDoNotShareConditions(t *testing.T) {
	base := NewAutomation("base").On(Daily(TimeOfDay(9, 0))).Do(noAction)

	strict, err := base.When(StateIs("light.a", "on")).Build()
	require.NoError(t, err)
	loose, err := base.Build()
	require.NoError(t, err)

	assert.NotNil(t, strict.condition)
	assert.Nil(t, loose.condition, "a sibling's condition must not attach here")
}

func TestFireRunsTheActionWhenConditionsHold(t *testing.T) {
	ran := make(chan struct{}, 1)
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(yes()).
		Do(func(context.Context, Run) error { ran <- struct{}{}; return nil }).
		MustBuild()

	require.True(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""))
	assertReceived(t, ran)
}

func TestFireSkipsWhenConditionsDoNotHold(t *testing.T) {
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(no()).
		Do(func(context.Context, Run) error { t.Error("action must not run"); return nil }).
		MustBuild()

	assert.False(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""))
}

func TestFireSkipsOnAnUnevaluableConditionByDefault(t *testing.T) {
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(broken()).
		Do(func(context.Context, Run) error { t.Error("action must not run"); return nil }).
		MustBuild()

	assert.False(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""),
		"an undecided condition defaults to not running")
}

func TestRunAnywayFiresDespiteAnUnevaluableCondition(t *testing.T) {
	ran := make(chan struct{}, 1)
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		When(broken()).
		OnConditionError(RunAnyway).
		Do(func(context.Context, Run) error { ran <- struct{}{}; return nil }).
		MustBuild()

	require.True(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""))
	assertReceived(t, ran)
}

// A failing action is logged and the automation stays live, rather than taking
// the process down with it.
func TestActionErrorsDoNotStopTheAutomation(t *testing.T) {
	ran := make(chan struct{}, 2)
	a := NewAutomation("a").
		On(Daily(TimeOfDay(9, 0))).
		Mode(ModeParallel).
		Do(func(context.Context, Run) error {
			ran <- struct{}{}
			return errors.New("service unavailable")
		}).
		MustBuild()

	require.True(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""))
	assertReceived(t, ran)
	require.True(t, a.fire(context.Background(), EvalContext{Clock: testClock()}, Run{}, ""))
	assertReceived(t, ran)
}

func assertReceived(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the action to run")
	}
}
