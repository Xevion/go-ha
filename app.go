package gomeassistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/dromara/carbon/v2"
	"github.com/gorilla/websocket"
	sunriseLib "github.com/nathan-osman/go-sunrise"

	"github.com/Workiva/go-datastructures/queue"
	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
	"github.com/Xevion/go-ha/types"
)

var ErrInvalidArgs = errors.New("invalid arguments provided")

type App struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// Wraps the ws connection with added mutex locking
	conn *connect.HAConnection

	httpClient *internal.HttpClient

	service *Service
	state   *StateImpl

	schedules         *queue.PriorityQueue
	intervals         *queue.PriorityQueue
	entityListeners   map[string][]*EntityListener
	entityListenersId int64
	eventListeners    map[string][]*EventListener
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
func validateHomeZone(state State, entityID string) error {
	entity, err := state.Get(entityID)
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

	conn, ctx, ctxCancel, err := connect.ConnectionFromUri(baseURL, request.HAAuthToken)
	if err != nil {
		return nil, err
	}

	httpClient := internal.NewHttpClient(ctx, baseURL, request.HAAuthToken)

	service := newService(conn)
	state, err := newState(httpClient, request.HomeZoneEntityId)
	if err != nil {
		return nil, err
	}

	// Validate home zone
	if err := validateHomeZone(state, request.HomeZoneEntityId); err != nil {
		return nil, err
	}

	return &App{
		conn:            conn,
		ctx:             ctx,
		ctxCancel:       ctxCancel,
		httpClient:      httpClient,
		service:         service,
		state:           state,
		schedules:       queue.NewPriorityQueue(100, false),
		intervals:       queue.NewPriorityQueue(100, false),
		entityListeners: map[string][]*EntityListener{},
		eventListeners:  map[string][]*EventListener{},
	}, nil
}

func (app *App) Cleanup() {
	if app.ctxCancel != nil {
		app.ctxCancel()
	}
}

// Close performs a clean shutdown of the application. It cancels the context, closes the WebSocket connection, and ensures all background processes are properly terminated.
func (app *App) Close() error {
	// Close WebSocket connection if it exists
	if app.conn != nil {
		deadline := time.Now().Add(10 * time.Second)
		err := app.conn.Conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
		if err != nil {
			slog.Warn("Error writing close message", "error", err)
			return err
		}

		// Close the WebSocket connection
		err = app.conn.Conn.Close()
		if err != nil {
			slog.Warn("Error closing WebSocket connection", "error", err)
			return err
		}
	}

	// Wait a short time for the WebSocket connection to close
	time.Sleep(500 * time.Millisecond)

	// Cancel context to signal all goroutines to stop
	if app.ctxCancel != nil {
		app.ctxCancel()
	}

	// Wait a short time for goroutines to finish
	// This allows for graceful shutdown of background processes
	time.Sleep(100 * time.Millisecond)

	return nil
}

func (app *App) RegisterSchedules(schedules ...DailySchedule) {
	for _, s := range schedules {
		// realStartTime already set for sunset/sunrise
		if s.isSunrise || s.isSunset {
			s.nextRunTime = getNextSunRiseOrSet(app, s.isSunrise, s.sunOffset).StdTime()
			app.schedules.Put()
			continue
		}

		now := carbon.Now()
		startTime := carbon.Now().SetTimeMilli(s.hour, s.minute, 0, 0)

		// advance first scheduled time by frequency until it is in the future
		if startTime.Lt(now) {
			startTime = startTime.AddDay()
		}

		s.nextRunTime = startTime.StdTime()
		app.schedules.Put(Item{
			Value:    s,
			Priority: float64(startTime.StdTime().Unix()),
		})
	}
}

func (app *App) RegisterIntervals(intervals ...Interval) {
	for _, i := range intervals {
		if i.frequency == 0 {
			slog.Error("A schedule must use either set frequency via Every()")
			panic(ErrInvalidArgs)
		}

		i.nextRunTime = internal.ParseTime(string(i.startTime)).StdTime()
		now := time.Now()
		for i.nextRunTime.Before(now) {
			i.nextRunTime = i.nextRunTime.Add(i.frequency)
		}
		app.intervals.Put(Item{
			Value:    i,
			Priority: float64(i.nextRunTime.Unix()),
		})
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
			} else {
				connect.SubscribeToEventType(eventType, app.conn, app.ctx)
				app.eventListeners[eventType] = []*EventListener{&evl}
			}
		}
	}
}

func getSunriseSunset(s *StateImpl, sunrise bool, dateToUse *carbon.Carbon, offset ...types.DurationString) *carbon.Carbon {
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

func getNextSunRiseOrSet(a *App, sunrise bool, offset ...types.DurationString) *carbon.Carbon {
	sunriseOrSunset := getSunriseSunset(a.state, sunrise, carbon.Now(), offset...)
	if sunriseOrSunset.Lt(carbon.Now()) {
		// if we're past today's sunset or sunrise (accounting for offset) then get tomorrows
		// as that's the next time the schedule will run
		sunriseOrSunset = getSunriseSunset(a.state, sunrise, carbon.Tomorrow(), offset...)
	}
	return sunriseOrSunset
}

func (app *App) Start() {
	slog.Info("Starting", "schedules", app.schedules.Len())
	slog.Info("Starting", "entity listeners", len(app.entityListeners))
	slog.Info("Starting", "event listeners", len(app.eventListeners))

	go runSchedules(app)
	go runIntervals(app)

	// subscribe to state_changed events
	id := internal.NextId()
	connect.SubscribeToStateChangedEvents(id, app.conn, app.ctx)
	app.entityListenersId = id

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

	// entity listeners and event listeners
	elChan := make(chan connect.ChannelMessage, 100) // Add buffer to prevent channel overflow
	go connect.ListenWebsocket(app.conn.Conn, elChan)

	for {
		select {
		case msg, ok := <-elChan:
			if !ok {
				slog.Info("WebSocket channel closed, stopping main loop")
				return
			}
			if app.entityListenersId == msg.Id {
				go callEntityListeners(app, msg.Raw)
			} else {
				go callEventListeners(app, msg)
			}
		case <-app.ctx.Done():
			slog.Info("Context cancelled, stopping main loop")
			return
		}
	}
}

func (app *App) GetService() *Service {
	return app.service
}

func (app *App) GetState() State {
	return app.state
}
