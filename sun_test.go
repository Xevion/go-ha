package ha

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sunEntity builds a sun.sun as Home Assistant publishes it, with RFC3339
// timestamps in its attributes.
func sunEntity(rising, setting time.Time) EntityState {
	return EntityState{
		EntityID: SunEntityID,
		State:    "above_horizon",
		Attributes: map[string]any{
			"next_rising":  rising.Format(time.RFC3339),
			"next_setting": setting.Format(time.RFC3339),
			"next_dawn":    rising.Add(-30 * time.Minute).Format(time.RFC3339),
			"next_dusk":    setting.Add(30 * time.Minute).Format(time.RFC3339),
		},
	}
}

func TestSunTriggersReadHomeAssistantsTimes(t *testing.T) {
	rising := time.Date(2026, 7, 20, 6, 30, 0, 0, time.Local)
	setting := time.Date(2026, 7, 19, 20, 33, 0, 0, time.Local)
	s := stateWith(sunEntity(rising, setting))

	for _, tt := range []struct {
		name string
		trig ScheduleTrigger
		want time.Time
	}{
		{"sunrise", Sunrise(), rising},
		{"sunset", Sunset(), setting},
		{"dawn", Dawn(), rising.Add(-30 * time.Minute)},
		{"dusk", Dusk(), setting.Add(30 * time.Minute)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.trig.(interface{ bind(StateReader) }).bind(s)

			got, ok := tt.trig.NextTime(time.Date(2026, 7, 19, 3, 0, 0, 0, time.Local))
			require.True(t, ok)
			assert.True(t, got.Equal(tt.want), "want %v, got %v", tt.want, got)
		})
	}
}

func TestSunTriggerAppliesOffset(t *testing.T) {
	setting := time.Date(2026, 7, 19, 20, 33, 0, 0, time.Local)
	s := stateWith(sunEntity(setting.Add(-14*time.Hour), setting))

	trig := Sunset(-30 * time.Minute)
	trig.(interface{ bind(StateReader) }).bind(s)

	got, ok := trig.NextTime(time.Date(2026, 7, 19, 3, 0, 0, 0, time.Local))
	require.True(t, ok)
	assert.True(t, got.Equal(setting.Add(-30*time.Minute)), "got %v", got)
}

// An unbound trigger has nothing to read. It must report that rather than
// answering with a zero time the scheduler would treat as due immediately.
func TestUnboundSunTriggerDoesNotFire(t *testing.T) {
	_, ok := Sunset().NextTime(time.Now())
	assert.False(t, ok)
}

func TestSunTriggerDoesNotFireWithoutTheSunEntity(t *testing.T) {
	trig := Sunset()
	trig.(interface{ bind(StateReader) }).bind(stateWith())

	_, ok := trig.NextTime(time.Now())
	assert.False(t, ok, "no sun.sun means no times to derive")
}

// The scheduler asks for the next occurrence immediately after firing, long
// before Home Assistant's updated attribute crosses the network. The answer is
// provisional and must at least be in the future, or the loop spins on it.
func TestSunTriggerAdvancesWhenTheAttributeHasNotRolledOver(t *testing.T) {
	setting := time.Date(2026, 7, 19, 20, 33, 0, 0, time.Local)
	s := stateWith(sunEntity(setting.Add(-14*time.Hour), setting))

	trig := Sunset()
	trig.(interface{ bind(StateReader) }).bind(s)

	next, ok := trig.NextTime(setting)
	require.True(t, ok)
	assert.True(t, next.After(setting), "requeueing from the fired instant must move forward")
}

// refresh is what makes the provisional answer above temporary: once Home
// Assistant republishes, the queued entry is corrected.
func TestRefreshCorrectsAProvisionalSunTime(t *testing.T) {
	setting := time.Date(2026, 7, 19, 20, 33, 0, 0, time.Local)
	s := stateWith(sunEntity(setting.Add(-14*time.Hour), setting))

	clock := testClock()
	clock.Set(setting)
	sched := newScheduler(clock)

	trig := Sunset()
	trig.(interface{ bind(StateReader) }).bind(s)
	require.True(t, sched.add(schedulerAdapter{trigger: trig}, func() {}))

	// Queued on the provisional day-later value.
	provisional := sched.peek().fireAt
	assert.True(t, provisional.After(setting))

	// Home Assistant publishes tomorrow's sunset, a minute earlier than a flat
	// day later, as the real one always is.
	tomorrow := setting.AddDate(0, 0, 1).Add(-time.Minute)
	s.cache.apply(sunEntity(tomorrow.Add(-14*time.Hour), tomorrow))

	assert.Equal(t, 1, sched.refresh(clock.Now()), "the queued entry must be re-derived")
	assert.True(t, sched.peek().fireAt.Equal(tomorrow),
		"want %v, got %v", tomorrow, sched.peek().fireAt)
}

// Only dynamic entries are re-derived. A fixed daily schedule that refresh
// moved would drift every time the sun updated.
func TestRefreshLeavesFixedSchedulesAlone(t *testing.T) {
	clock := testClock()
	sched := newScheduler(clock)
	require.True(t, sched.add(schedulerAdapter{trigger: Daily(TimeOfDay(9, 0))}, func() {}))

	before := sched.peek().fireAt
	assert.Equal(t, 0, sched.refresh(clock.Now()))
	assert.True(t, sched.peek().fireAt.Equal(before))
}

func TestSunAutomationRegistersAndBinds(t *testing.T) {
	setting := time.Date(2026, 7, 19, 20, 33, 0, 0, time.Local)
	app := testApp(sunEntity(setting.Add(-14*time.Hour), setting))
	app.clock.(interface{ Set(time.Time) }).Set(time.Date(2026, 7, 19, 3, 0, 0, 0, time.Local))

	ran := make(chan struct{}, 1)
	a := NewAutomation("porch").
		On(Sunset(-15 * time.Minute)).
		Do(func(context.Context, Run) error { ran <- struct{}{}; return nil }).
		MustBuild()

	require.NoError(t, app.RegisterAutomations(a))
	require.Equal(t, 1, app.schedules.len(), "registration must bind and queue the trigger")

	assert.True(t, app.schedules.peek().fireAt.Equal(setting.Add(-15*time.Minute)))
}
