package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
	"github.com/Xevion/go-ha/types"
)

var (
	// ErrInvalidArgs reports a malformed NewAppRequest.
	ErrInvalidArgs = errors.New("invalid arguments provided")

	// ErrConnectionAbandoned reports that the client gave up re-establishing
	// the connection, so Start returned without being asked to.
	ErrConnectionAbandoned = errors.New("connection abandoned")

	// ErrNotRunning reports Start called twice, or after Close.
	ErrNotRunning = errors.New("app is not runnable")
)

type App struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// Owns the websocket connection, and re-establishes it when it drops.
	client *connect.Client

	httpClient *internal.HttpClient
	clock      Clock

	service *Service
	state   *state

	schedules *scheduler
	intervals *scheduler

	// registryMu guards the automation registry. Dispatch runs on the client's
	// worker goroutines, which are live from the moment the connection is up,
	// so registration cannot assume it has the maps to itself.
	registryMu sync.RWMutex

	// automations maps an event type to the automations waiting on it.
	automations map[string][]binding

	// runners holds every registered automation's runner, deduplicated because
	// an automation with several triggers registers once per trigger. Shutdown
	// waits on these so a run in flight finishes its service calls.
	runners map[*runner]struct{}

	// rescheduled wakes the schedule loop when a dynamic trigger's time moves.
	// A refreshed sun time can be earlier than the one the loop is sleeping
	// on, and it would otherwise wake too late to fire it.
	rescheduled chan struct{}

	// loops tracks the schedule and interval goroutines. They admit runs of
	// their own, so shutdown has to join them before waiting on any runner: a
	// WaitGroup may not be raised from zero while a Wait on it is in flight.
	loops sync.WaitGroup

	// starting guards Start against being entered twice, which would double the
	// loops and race Close's wait on them.
	starting atomic.Bool

	// started gates listener dispatch. The state_changed subscription exists
	// from construction so the cache stays current, but listeners must not run
	// before Start has taken its startup pass.
	started atomic.Bool
}

// NewApp establishes the WebSocket connection and returns an object you can use to register schedules and listeners.
func NewApp(request types.NewAppRequest) (*App, error) {
	if request.URL == "" || request.HAAuthToken == "" {
		return nil, fmt.Errorf("%w: URL and HAAuthToken are both required", ErrInvalidArgs)
	}

	baseURL, err := url.Parse(request.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL %q: %w", request.URL, err)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	httpClient := internal.NewHttpClient(ctx, baseURL, request.HAAuthToken)

	var clock Clock = internal.RealClock{}
	if request.Clock != nil {
		clock = request.Clock
	}

	state := newState(httpClient)

	client, err := connect.NewClient(baseURL, request.HAAuthToken, connect.Options{
		QueueSize:    request.Connection.QueueSize,
		Workers:      request.Connection.Workers,
		PingInterval: request.Connection.PingInterval,
		// Every connection starts with a fresh snapshot. Anything that changed
		// while the stream was down was never delivered.
		OnConnected: func() {
			if err := state.seed(); err != nil {
				slog.Error("Failed to load entity states", "error", err)
			}
		},
	})
	if err != nil {
		ctxCancel()
		return nil, err
	}

	app := &App{
		client:      client,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		httpClient:  httpClient,
		clock:       clock,
		service:     newService(client),
		state:       state,
		schedules:   newScheduler(clock),
		intervals:   newScheduler(clock),
		automations: map[string][]binding{},
		runners:     map[*runner]struct{}{},
		rescheduled: make(chan struct{}, 1),
	}

	// Subscribing before connecting, so the replay that runs on every
	// connection establishes it before the snapshot is taken. Taking the
	// snapshot first would lose whatever changed in between.
	if err := client.Subscribe(
		connect.Subscription{EventType: "state_changed"},
		app.onStateChanged,
	); err != nil {
		ctxCancel()
		return nil, err
	}

	if err := client.Connect(ctx); err != nil {
		ctxCancel()
		return nil, err
	}

	return app, nil
}

// refreshSunSchedules re-derives sun-backed schedules when Home Assistant
// republishes their times, which it does as each solar event passes.
func (app *App) refreshSunSchedules(raw []byte) {
	if !bytesMentionSunEntity(raw) {
		return
	}
	if app.schedules.refresh(app.clock.Now()) == 0 {
		return
	}

	// Non-blocking: the loop only needs to know something moved, and a second
	// notification while one is already pending would tell it nothing new.
	select {
	case app.rescheduled <- struct{}{}:
	default:
	}
}

// bytesMentionSunEntity is a cheap reject before decoding. Every state_changed
// event reaches here, and almost none of them are the sun.
func bytesMentionSunEntity(raw []byte) bool {
	return bytes.Contains(raw, []byte(SunEntityID))
}

// onEvent dispatches an event to the automations waiting on it. Like the
// state_changed path, it holds off until Start: an automation must not fire
// before the app it belongs to is running.
func (app *App) onEvent(msg connect.Message) {
	if !app.started.Load() {
		return
	}
	app.dispatchEvent(msg.Raw)
}

// onStateChanged keeps the cache current and, once the app has started, runs
// the listeners watching the entity.
func (app *App) onStateChanged(msg connect.Message) {
	app.state.applyEvent(msg.Raw)
	app.refreshSunSchedules(msg.Raw)

	// Before Start the cache is still worth maintaining, but automations must
	// not fire until the app is running.
	if app.started.Load() {
		app.dispatchEvent(msg.Raw)
	}
}

// Cleanup shuts the application down.
//
// Deprecated: use Close, which reports whether shutdown succeeded.
func (app *App) Cleanup() {
	_ = app.Close()
}

// Close performs a clean shutdown: it stops the background goroutines, closes
// the connection, and waits for both to finish.
func (app *App) Close() error {
	if app.ctxCancel != nil {
		app.ctxCancel()
	}

	var closeErr error
	if app.client != nil {
		// Close waits for the client's goroutines, so shutdown no longer
		// guesses at how long they need with a pair of sleeps.
		if err := app.client.Close(); err != nil {
			closeErr = fmt.Errorf("closing connection: %w", err)
		}
	}

	// This runs after the client has stopped, not before: a handler still in
	// flight arms a timer from a worker goroutine, and would otherwise slip one
	// in behind a pass that had already walked past it.
	app.registryMu.RLock()
	runners := make([]*runner, 0, len(app.runners))
	for r := range app.runners {
		runners = append(runners, r)
	}
	// Same reasoning as the listener timers: a trigger waiting out a For
	// duration would otherwise fire into a closed connection.
	for _, bindings := range app.automations {
		for _, b := range bindings {
			b.pending.stop()
		}
	}
	app.registryMu.RUnlock()

	// The schedule and interval loops admit runs of their own, so they have to
	// be quiescent before any runner is waited on. Otherwise a loop that has
	// already passed its cancellation check admits a run behind the pass, and
	// raising a WaitGroup from zero under an in-flight Wait is a hard throw.
	app.loops.Wait()

	// Automation runs hold a context derived from the app's, which is already
	// cancelled, so this waits out work that is winding down rather than work
	// that is still starting.
	for _, r := range runners {
		r.wait()
	}

	return closeErr
}

// Start runs the app until its context is cancelled or the client abandons
// reconnection. It returns the reason it stopped: nil for a clean shutdown,
// ErrConnectionAbandoned when the connection could not be recovered.
//
// Calling it twice, or after Close, is a no-op returning ErrNotRunning.
func (app *App) Start() error {
	if !app.starting.CompareAndSwap(false, true) {
		return ErrNotRunning
	}
	if app.ctx.Err() != nil {
		return ErrNotRunning
	}

	app.registryMu.RLock()
	eventTypes := len(app.automations)
	app.registryMu.RUnlock()

	slog.Info("Starting",
		"version", internal.Version,
		"schedules", app.schedules.len(),
		"intervals", app.intervals.len(),
		"event_types", eventTypes,
	)

	// Separate channels: a wake meant for the schedules loop would otherwise be
	// consumed by the intervals loop, which has no dynamic triggers to re-read,
	// and the schedule that actually moved would sleep through it.
	app.loops.Add(2)
	go func() { defer app.loops.Done(); app.schedules.run(app.ctx, app.rescheduled, "schedules") }()
	go func() { defer app.loops.Done(); app.intervals.run(app.ctx, nil, "intervals") }()

	// Opening the gate last, so nothing fires before the loops are up.
	app.started.Store(true)

	select {
	case <-app.ctx.Done():
		slog.Info("Context cancelled, stopping")
		return nil
	case <-app.client.Done():
		// The client gave up reconnecting, so blocking on our own context
		// would leave the app alive but permanently deaf. Cancelling also
		// stops the schedule and interval loops, which would otherwise keep
		// firing callbacks whose service calls have nowhere to go.
		slog.Error("Connection abandoned, stopping")
		app.ctxCancel()
		return ErrConnectionAbandoned
	}
}

func (app *App) Services() *Service {
	return app.service
}

func (app *App) State() StateReader {
	return app.state
}
