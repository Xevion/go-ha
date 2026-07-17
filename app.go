package ha

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Workiva/go-datastructures/queue"
	"github.com/dromara/carbon/v2"
	sunriseLib "github.com/nathan-osman/go-sunrise"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/Xevion/go-ha/types"
)

var ErrInvalidArgs = errors.New("invalid arguments provided")

type App struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// Owns the websocket connection, and re-establishes it when it drops.
	client *connect.Client

	httpClient *internal.HttpClient
	clock      internal.Clock

	service *Service
	state   *state

	schedules *scheduler
	intervals *scheduler

	// Guards the two listener maps. Dispatch runs on the client's worker
	// goroutines, which are live from the moment the connection is up, so
	// registration can no longer assume it has the maps to itself.
	listenersMu     sync.RWMutex
	entityListeners map[string][]*EntityListener
	eventListeners  map[string][]*EventListener

	// automations maps an event type to the automations waiting on it.
	automations map[string][]binding

	// runners holds every registered automation's runner, deduplicated because
	// an automation with several triggers registers once per trigger. Shutdown
	// waits on these so a run in flight finishes its service calls.
	runners map[*runner]struct{}

	// loops tracks the schedule and interval goroutines. They admit runs of
	// their own, so shutdown has to join them before waiting on any runner: a
	// WaitGroup may not be raised from zero while a Wait on it is in flight.
	loops sync.WaitGroup

	// started gates listener dispatch. The state_changed subscription exists
	// from construction so the cache stays current, but listeners must not run
	// before Start has taken its startup pass.
	started atomic.Bool
}

type Item types.Item

func (i Item) Compare(other queue.Item) int {
	if i.Priority > other.(Item).Priority {
		return 1
	} else if i.Priority == other.(Item).Priority {
		return 0
	}
	return -1
}

// validateHomeZone verifies that the home zone entity exists and has latitude/longitude.
func validateHomeZone(s StateReader, entityID string) error {
	entity, err := s.Get(entityID)
	if err != nil {
		return fmt.Errorf("home zone entity '%s' not found: %w", entityID, err)
	}

	// Ensure it's a zone entity
	if !strings.HasPrefix(entityID, "zone.") {
		return fmt.Errorf("entity '%s' is not a zone entity (must start with zone.)", entityID)
	}

	// Verify it has latitude and longitude
	if entity.Attributes == nil {
		return fmt.Errorf("home zone entity '%s' has no attributes", entityID)
	} else if entity.Attributes["latitude"] == nil {
		return fmt.Errorf("home zone entity '%s' missing latitude attribute", entityID)
	} else if entity.Attributes["longitude"] == nil {
		return fmt.Errorf("home zone entity '%s' missing longitude attribute", entityID)
	}

	return nil
}

// NewApp establishes the WebSocket connection and returns an object you can use to register schedules and listeners.
func NewApp(request types.NewAppRequest) (*App, error) {
	if (request.URL == "" && request.IpAddress == "") || request.HAAuthToken == "" {
		slog.Error("URL and HAAuthToken are required arguments in NewAppRequest")
		return nil, ErrInvalidArgs
	}

	// Set default home zone if not provided
	if request.HomeZoneEntityId == "" {
		request.HomeZoneEntityId = "zone.home"
	}

	baseURL := &url.URL{}

	if request.URL != "" {
		var err error
		baseURL, err = url.Parse(request.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse URL: %w", err)
		}
	}

	ctx, ctxCancel := context.WithCancel(context.Background())

	httpClient := internal.NewHttpClient(ctx, baseURL, request.HAAuthToken)

	clock := internal.RealClock{}
	state, err := newState(httpClient, request.HomeZoneEntityId)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// Validate home zone
	if err := validateHomeZone(state, request.HomeZoneEntityId); err != nil {
		ctxCancel()
		return nil, err
	}

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
		client:          client,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		httpClient:      httpClient,
		clock:           clock,
		service:         newService(client),
		state:           state,
		schedules:       newScheduler(clock),
		intervals:       newScheduler(clock),
		entityListeners: map[string][]*EntityListener{},
		eventListeners:  map[string][]*EventListener{},
		automations:     map[string][]binding{},
		runners:         map[*runner]struct{}{},
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

	// Before Start the cache is still worth maintaining, but the startup pass
	// has not run yet and owns which listeners it marks completed.
	if app.started.Load() {
		callEntityListeners(app, msg.Raw)
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

	// Cancel any listener still waiting out a Duration(). Left armed, it fires
	// after shutdown and runs a callback whose service calls have nowhere to go.
	//
	// This runs after the client has stopped, not before: a handler still in
	// flight arms its timer from a worker goroutine, and would otherwise slip
	// one in behind a pass that had already walked past it.
	app.listenersMu.RLock()
	for _, etls := range app.entityListeners {
		for _, etl := range etls {
			etl.runtime.disarm()
		}
	}
	runners := make([]*runner, 0, len(app.runners))
	for r := range app.runners {
		runners = append(runners, r)
	}
	app.listenersMu.RUnlock()

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

func (app *App) RegisterSchedules(schedules ...DailySchedule) {
	for _, s := range schedules {
		if s.specErr != nil {
			slog.Error("Invalid schedule", "error", s.specErr)
			panic(s.specErr)
		}
		if s.spec == nil {
			slog.Error("A schedule must set a time via At(), Sunrise() or Sunset()")
			panic(ErrInvalidArgs)
		}

		trigger, err := s.spec.Resolve(app.location())
		if err != nil {
			slog.Error("Could not resolve schedule trigger", "error", err)
			panic(err)
		}

		app.schedules.add(trigger, func() { s.maybeRunCallback(app) })
	}
}

// location reports the home zone coordinates that sun triggers resolve against.
func (app *App) location() scheduling.Location {
	return scheduling.Location{
		Latitude:  app.state.latitude,
		Longitude: app.state.longitude,
	}
}

func (app *App) RegisterIntervals(intervals ...Interval) {
	for _, i := range intervals {
		if i.triggerErr != nil {
			slog.Error("Invalid interval", "error", i.triggerErr)
			panic(i.triggerErr)
		}
		if i.trigger == nil {
			slog.Error("An interval must set a frequency via Every()")
			panic(ErrInvalidArgs)
		}

		app.intervals.add(i.trigger, func() { i.maybeRunCallback(app) })
	}
}

func (app *App) RegisterEntityListeners(etls ...EntityListener) {
	app.listenersMu.Lock()
	defer app.listenersMu.Unlock()

	for _, etl := range etls {
		etl := etl
		if etl.delay != 0 && etl.toState == "" {
			slog.Error("EntityListener error: you have to use ToState() when using Duration()")
			panic(ErrInvalidArgs)
		}

		for _, entity := range etl.entityIds {
			if elList, ok := app.entityListeners[entity]; ok {
				app.entityListeners[entity] = append(elList, &etl)
			} else {
				app.entityListeners[entity] = []*EntityListener{&etl}
			}
		}
	}
}

func (app *App) RegisterEventListeners(evls ...EventListener) {
	var fresh []string

	app.listenersMu.Lock()
	for _, evl := range evls {
		evl := evl
		for _, eventType := range evl.eventTypes {
			if elList, ok := app.eventListeners[eventType]; ok {
				app.eventListeners[eventType] = append(elList, &evl)
				continue
			}
			app.eventListeners[eventType] = []*EventListener{&evl}
			fresh = append(fresh, eventType)
		}
	}
	app.listenersMu.Unlock()

	// Subscribing only once the map is published and unlocked. Home Assistant
	// starts delivering as soon as the request lands, and those events arrive
	// on a worker goroutine that has to read the very map being built here.
	for _, eventType := range fresh {
		if err := app.client.Subscribe(
			connect.Subscription{EventType: eventType},
			func(msg connect.Message) { callEventListeners(app, msg) },
		); err != nil {
			slog.Error("Failed to subscribe to event type", "event_type", eventType, "error", err)
		}
	}
}

func getSunriseSunset(s *state, sunrise bool, dateToUse *carbon.Carbon, offset ...types.DurationString) *carbon.Carbon {
	date := dateToUse.StdTime()
	rise, set := sunriseLib.SunriseSunset(s.latitude, s.longitude, date.Year(), date.Month(), date.Day())
	rise, set = rise.Local(), set.Local()

	val := set
	printString := "Sunset"
	if sunrise {
		val = rise
		printString = "Sunrise"
	}

	setOrRiseToday := carbon.Parse(val.String())

	var t time.Duration
	var err error
	if len(offset) == 1 {
		t, err = time.ParseDuration(string(offset[0]))
		if err != nil {
			parsingErr := fmt.Errorf("could not parse offset passed to %s: \"%s\": %w", printString, offset[0], err)
			slog.Error(parsingErr.Error())
			panic(parsingErr)
		}
	}

	// add offset if set, this code works for negative values too
	if t.Microseconds() != 0 {
		setOrRiseToday = setOrRiseToday.AddMinutes(int(t.Minutes()))
	}

	return setOrRiseToday
}

func (app *App) Start() {
	app.listenersMu.RLock()
	entityCount, eventCount := len(app.entityListeners), len(app.eventListeners)
	app.listenersMu.RUnlock()

	slog.Info("Starting",
		"version", Version,
		"schedules", app.schedules.len(),
		"intervals", app.intervals.len(),
		"entity_listeners", entityCount,
		"event_listeners", eventCount,
	)

	app.loops.Add(2)
	go func() { defer app.loops.Done(); runSchedules(app) }()
	go func() { defer app.loops.Done(); runIntervals(app) }()

	// Run entity listeners startup
	app.listenersMu.RLock()
	for eid, etls := range app.entityListeners {
		for _, etl := range etls {
			// ensure each ETL only runs once, even if
			// it listens to multiple entities
			if etl.runOnStartup && !etl.runOnStartupCompleted {
				entityState, err := app.state.Get(eid)
				if err != nil {
					slog.Warn("Failed to get entity state \"", eid, "\" during startup, skipping RunOnStartup")
				}

				etl.runOnStartupCompleted = true
				go etl.callback(app.service, app.state, EntityData{
					TriggerEntityId: eid,
					FromState:       entityState.State,
					FromAttributes:  entityState.Attributes,
					ToState:         entityState.State,
					ToAttributes:    entityState.Attributes,
					LastChanged:     entityState.LastChanged,
				})
			}
		}
	}
	app.listenersMu.RUnlock()

	// Opening the gate last, so the startup pass owns runOnStartupCompleted
	// outright rather than racing events already arriving from the stream.
	app.started.Store(true)

	// Dispatch belongs to the client now: it routes each message to the
	// subscription that asked for it, so there is nothing left to demultiplex
	// here and no id to compare against.
	select {
	case <-app.ctx.Done():
		slog.Info("Context cancelled, stopping")
	case <-app.client.Done():
		// The client gave up reconnecting, so blocking on our own context
		// would leave the app alive but permanently deaf. Cancelling also
		// stops the schedule and interval goroutines, which would otherwise
		// keep firing callbacks whose service calls have nowhere to go.
		slog.Error("Connection abandoned, stopping")
		app.ctxCancel()
	}
}

func (app *App) Services() *Service {
	return app.service
}

func (app *App) State() StateReader {
	return app.state
}
