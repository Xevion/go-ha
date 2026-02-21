package ha

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
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

	schedules       *scheduler
	intervals       *scheduler
	entityListeners map[string][]*EntityListener
	eventListeners  map[string][]*EventListener
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

	client, err := connect.NewClient(baseURL, request.HAAuthToken, connect.Options{
		QueueSize:    request.Connection.QueueSize,
		Workers:      request.Connection.Workers,
		PingInterval: request.Connection.PingInterval,
	})
	if err != nil {
		ctxCancel()
		return nil, err
	}
	if err := client.Connect(ctx); err != nil {
		ctxCancel()
		return nil, err
	}

	httpClient := internal.NewHttpClient(ctx, baseURL, request.HAAuthToken)

	clock := internal.RealClock{}
	service := newService(client)
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

	return &App{
		client:          client,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		httpClient:      httpClient,
		clock:           clock,
		service:         service,
		state:           state,
		schedules:       newScheduler(clock),
		intervals:       newScheduler(clock),
		entityListeners: map[string][]*EntityListener{},
		eventListeners:  map[string][]*EventListener{},
	}, nil
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

	if app.client == nil {
		return nil
	}
	// Close waits for the client's goroutines, so shutdown no longer guesses at
	// how long they need with a pair of sleeps.
	if err := app.client.Close(); err != nil {
		return fmt.Errorf("closing connection: %w", err)
	}
	return nil
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
	for _, evl := range evls {
		evl := evl
		for _, eventType := range evl.eventTypes {
			if elList, ok := app.eventListeners[eventType]; ok {
				app.eventListeners[eventType] = append(elList, &evl)
				continue
			}

			app.eventListeners[eventType] = []*EventListener{&evl}
			if err := app.client.Subscribe(
				connect.Subscription{EventType: eventType},
				func(msg connect.Message) { callEventListeners(app, msg) },
			); err != nil {
				slog.Error("Failed to subscribe to event type", "event_type", eventType, "error", err)
			}
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
	slog.Info("Starting",
		"version", Version,
		"schedules", app.schedules.len(),
		"intervals", app.intervals.len(),
		"entity_listeners", len(app.entityListeners),
		"event_listeners", len(app.eventListeners),
	)

	go runSchedules(app)
	go runIntervals(app)

	if err := app.client.Subscribe(
		connect.Subscription{EventType: "state_changed"},
		func(msg connect.Message) { callEntityListeners(app, msg.Raw) },
	); err != nil {
		slog.Error("Failed to subscribe to state changes", "error", err)
	}

	// Run entity listeners startup
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

	// Dispatch belongs to the client now: it routes each message to the
	// subscription that asked for it, so there is nothing left to demultiplex
	// here and no id to compare against.
	<-app.ctx.Done()
	slog.Info("Context cancelled, stopping")
}

func (app *App) Services() *Service {
	return app.service
}

func (app *App) State() StateReader {
	return app.state
}
