package ha

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/connect"
	"github.com/Xevion/go-ha/types"
)

type EventListener struct {
	eventTypes   []string
	callback     EventListenerCallback
	betweenStart string
	betweenEnd   string
	throttle     time.Duration

	// Shared by every copy this listener's builder chain produces, and by every
	// worker that dispatches to it.
	runtime *listenerRuntime

	dateFilter

	enabledEntities  []internal.EnabledDisabledInfo
	disabledEntities []internal.EnabledDisabledInfo
}

type EventListenerCallback func(*Service, StateReader, EventData)

type EventData struct {
	Type         string
	RawEventJSON []byte
}

func NewEventListener() eventListenerBuilder1 {
	return eventListenerBuilder1{EventListener{
		runtime: newListenerRuntime(),
	}}
}

type eventListenerBuilder1 struct {
	eventListener EventListener
}

func (b eventListenerBuilder1) EventTypes(ets ...string) eventListenerBuilder2 {
	b.eventListener.eventTypes = ets
	return eventListenerBuilder2(b)
}

type eventListenerBuilder2 struct {
	eventListener EventListener
}

func (b eventListenerBuilder2) Call(callback EventListenerCallback) eventListenerBuilder3 {
	b.eventListener.callback = callback
	return eventListenerBuilder3(b)
}

type eventListenerBuilder3 struct {
	eventListener EventListener
}

func (b eventListenerBuilder3) OnlyBetween(start string, end string) eventListenerBuilder3 {
	b.eventListener.betweenStart = start
	b.eventListener.betweenEnd = end
	return b
}

func (b eventListenerBuilder3) OnlyAfter(start string) eventListenerBuilder3 {
	b.eventListener.betweenStart = start
	return b
}

func (b eventListenerBuilder3) OnlyBefore(end string) eventListenerBuilder3 {
	b.eventListener.betweenEnd = end
	return b
}

func (b eventListenerBuilder3) Throttle(s types.DurationString) eventListenerBuilder3 {
	d := internal.ParseDuration(string(s))
	b.eventListener.throttle = d
	return b
}

func (b eventListenerBuilder3) ExceptionDates(t time.Time, tl ...time.Time) eventListenerBuilder3 {
	b.eventListener.addExceptions(t, tl...)
	return b
}

func (b eventListenerBuilder3) ExceptionRange(start, end time.Time) eventListenerBuilder3 {
	b.eventListener.addRange(start, end)
	return b
}

// EnabledWhen enables this listener only when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the listener runs if {runOnNetworkError} is true.
func (b eventListenerBuilder3) EnabledWhen(entityId, state string, runOnNetworkError bool) eventListenerBuilder3 {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in eventListener EnabledWhen entityId='%s' state='%s' runOnNetworkError='%t'", entityId, state, runOnNetworkError))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	b.eventListener.enabledEntities = append(b.eventListener.enabledEntities, i)
	return b
}

// DisabledWhen disables this listener when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the listener runs if {runOnNetworkError} is true.
func (b eventListenerBuilder3) DisabledWhen(entityId, state string, runOnNetworkError bool) eventListenerBuilder3 {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in eventListener EnabledWhen entityId='%s' state='%s' runOnNetworkError='%t'", entityId, state, runOnNetworkError))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	b.eventListener.disabledEntities = append(b.eventListener.disabledEntities, i)
	return b
}

func (b eventListenerBuilder3) Build() EventListener {
	return b.eventListener
}

type BaseEventMsg struct {
	Event struct {
		EventType string `json:"event_type"`
	} `json:"event"`
}

func callEventListeners(app *App, msg connect.Message) {
	baseEventMsg := BaseEventMsg{}
	_ = json.Unmarshal(msg.Raw, &baseEventMsg)
	app.listenersMu.RLock()
	listeners, ok := app.eventListeners[baseEventMsg.Event.EventType]
	app.listenersMu.RUnlock()
	if !ok {
		// no listeners registered for this event type
		return
	}

	for _, l := range listeners {
		// Check conditions
		if c := CheckWithinTimeRange(app.clock, l.betweenStart, l.betweenEnd); c.fail {
			continue
		}
		// A cheap reject before the checks below, which reach Home Assistant
		// over HTTP. The binding decision is made by claim, at fire time.
		if l.runtime.throttled(app.clock, l.throttle) {
			continue
		}
		if c := CheckExceptionDates(app.clock, l.exceptionDates); c.fail {
			continue
		}
		if c := CheckExceptionRanges(app.clock, l.exceptionRanges); c.fail {
			continue
		}
		if c := CheckEnabledEntity(app.state, l.enabledEntities); c.fail {
			continue
		}
		if c := CheckDisabledEntity(app.state, l.disabledEntities); c.fail {
			continue
		}

		eventData := EventData{
			Type:         baseEventMsg.Event.EventType,
			RawEventJSON: msg.Raw,
		}
		if !l.runtime.claim(app.clock, l.throttle) {
			continue
		}
		go l.callback(app.service, app.state, eventData)
	}
}
