package ha

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Xevion/go-ha/internal"
)

// Close must join the schedule and interval loops before waiting on any
// runner. Those loops check for cancellation only at the top of their pass, so
// one that has already gone past the check will admit a run behind it. Waiting
// on a runner whose count that run is about to raise is also WaitGroup misuse,
// which the runtime throws on rather than tolerating.
func TestCloseWaitsOutScheduleTriggeredRuns(t *testing.T) {
	var afterClose atomic.Int64
	var closed atomic.Bool

	for range 200 {
		clock := internal.RealClock{}
		ctx, cancel := context.WithCancel(context.Background())
		app := &App{
			ctx:         ctx,
			ctxCancel:   cancel,
			clock:       clock,
			state:       stateWith(),
			schedules:   newScheduler(clock),
			intervals:   newScheduler(clock),
			automations: map[string][]binding{},
			runners:     map[*runner]struct{}{},
			rescheduled: make(chan struct{}, 1),
		}

		a := NewAutomation("fast").
			On(Every(time.Millisecond)).
			Mode(ModeParallel).
			Do(func(context.Context, Run) error {
				if closed.Load() {
					afterClose.Add(1)
				}
				return nil
			}).
			MustBuild()
		require.NoError(t, app.RegisterAutomations(a))

		// Mirrors what Start does, which is where the loops are registered.
		app.loops.Add(1)
		go func() { defer app.loops.Done(); app.schedules.run(app.ctx, app.rescheduled, "schedules") }()

		time.Sleep(3 * time.Millisecond)

		require.NoError(t, app.Close())
		closed.Store(true)
		time.Sleep(3 * time.Millisecond)
		closed.Store(false)
	}

	assert.Zero(t, afterClose.Load(), "an automation ran after Close reported a clean shutdown")
}
