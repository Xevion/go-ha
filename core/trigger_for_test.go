package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForWaitsOutTheDuration(t *testing.T) {
	app := testApp()

	fired := make(chan struct{}, 1)
	a := NewAutomation("away").
		On(StateChanged("binary_sensor.motion").To("off").For(50 * time.Millisecond)).
		Do(func(context.Context, Run) error { fired <- struct{}{}; return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "on", "off"))
	assert.Empty(t, fired, "the state has not been held long enough yet")

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("the run never happened")
	}
}

// A change away from the awaited state cancels the wait. This is the whole
// point of For: motion stopping for five minutes, not motion stopping once.
func TestForIsCancelledWhenTheStateMovesAway(t *testing.T) {
	app := testApp()

	a := NewAutomation("away").
		On(StateChanged("binary_sensor.motion").To("off").For(50 * time.Millisecond)).
		Do(func(context.Context, Run) error { t.Error("the wait should have been cancelled"); return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "on", "off"))
	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "off", "on"))

	time.Sleep(150 * time.Millisecond)
	a.runtime.wait()
}

// One automation can watch several entities, and each holds its own wait.
func TestForKeepsAWaitPerEntity(t *testing.T) {
	app := testApp()

	fired := make(chan string, 4)
	a := NewAutomation("away").
		On(StateChanged("binary_sensor.a", "binary_sensor.b").To("off").For(50 * time.Millisecond)).
		Mode(ModeParallel).
		Do(func(_ context.Context, run Run) error { fired <- run.Event.EntityID; return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.a", "on", "off"))
	app.dispatchEvent(stateChangedJSON("binary_sensor.b", "on", "off"))

	// Cancelling one must leave the other's wait running.
	app.dispatchEvent(stateChangedJSON("binary_sensor.a", "off", "on"))

	time.Sleep(200 * time.Millisecond)
	a.runtime.wait()

	close(fired)
	var got []string
	for e := range fired {
		got = append(got, e)
	}
	assert.Equal(t, []string{"binary_sensor.b"}, got)
}

func TestCloseCancelsPendingForWaits(t *testing.T) {
	app := testApp()

	a := NewAutomation("away").
		On(StateChanged("binary_sensor.motion").To("off").For(50 * time.Millisecond)).
		Do(func(context.Context, Run) error { t.Error("a wait must not survive shutdown"); return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	app.dispatchEvent(stateChangedJSON("binary_sensor.motion", "on", "off"))
	require.NoError(t, app.Close())

	time.Sleep(150 * time.Millisecond)
}

func TestAtStartupFiresOnceOnly(t *testing.T) {
	trig := AtStartup()
	now := time.Date(2026, 7, 19, 3, 0, 0, 0, time.Local)

	first, ok := trig.NextTime(now)
	require.True(t, ok)
	assert.True(t, first.Equal(now), "startup is due immediately")

	_, ok = trig.NextTime(now)
	assert.False(t, ok, "it must not fire a second time")
}

func TestAtStartupRunsThroughTheScheduler(t *testing.T) {
	app := testApp()

	fired := make(chan struct{}, 2)
	a := NewAutomation("boot").
		On(AtStartup()).
		Do(func(context.Context, Run) error { fired <- struct{}{}; return nil }).
		MustBuild()
	require.NoError(t, app.RegisterAutomations(a))

	assert.Equal(t, 1, app.schedules.runDue(app.clock.Now()))
	a.runtime.wait()
	assert.Len(t, fired, 1)

	// Retired after firing, so a later pass finds nothing.
	assert.Equal(t, 0, app.schedules.runDue(app.clock.Now()))
}

// Stop cannot recall a timer whose callback has already begun. Such a callback
// used to delete the map entry belonging to its own replacement, leaving that
// replacement untracked and beyond the reach of disarm and stop, so a wait
// could fire after shutdown.
func TestSupersededWaitDoesNotOrphanItsReplacement(t *testing.T) {
	p := newPendingRuns()

	var ranB atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})

	p.arm("light.a", time.Millisecond, func() {})

	// Stand in for the first callback interleaving: past Stop, about to take
	// the lock and clear its entry.
	go func() {
		close(entered)
		<-release
		p.mu.Lock()
		if p.gen["light.a"] == 1 {
			delete(p.timers, "light.a")
		}
		p.mu.Unlock()
	}()
	<-entered

	p.arm("light.a", 60*time.Millisecond, func() { ranB.Store(true) })
	close(release)
	time.Sleep(10 * time.Millisecond)

	p.disarm("light.a")
	time.Sleep(120 * time.Millisecond)

	assert.False(t, ranB.Load(), "the replacement fired despite being disarmed")
}
