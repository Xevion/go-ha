package gomeassistant

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dromara/carbon/v2"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/types"
)

type EntityListener struct {
	entityIds []string
	callback  EntityListenerCallback
	fromState string
	toState   string
	throttle  time.Duration
	lastRan   *carbon.Carbon

	betweenStart string
	betweenEnd   string

	delay      time.Duration
	delayTimer *time.Timer

	exceptionDates  []time.Time
	exceptionRanges []types.TimeRange

	runOnStartup          bool
	runOnStartupCompleted bool

	enabledEntities  []internal.EnabledDisabledInfo
	disabledEntities []internal.EnabledDisabledInfo
}

type EntityListenerCallback func(*Service, State, EntityData)

type EntityData struct {
	TriggerEntityId string
	FromState       string
	FromAttributes  map[string]any
	ToState         string
	ToAttributes    map[string]any
	LastChanged     time.Time
}

type stateChangedMsg struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Event struct {
		Data struct {
			EntityID string   `json:"entity_id"`
			NewState msgState `json:"new_state"`
			OldState msgState `json:"old_state"`
		} `json:"data"`
		EventType string `json:"event_type"`
		Origin    string `json:"origin"`
	} `json:"event"`
}

type msgState struct {
	EntityID    string         `json:"entity_id"`
	LastChanged time.Time      `json:"last_changed"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
}

func NewEntityListener() elBuilder1 {
	return elBuilder1{EntityListener{
		lastRan: carbon.Now().StartOfCentury(),
	}}
}

type elBuilder1 struct {
	entityListener EntityListener
}

func (b elBuilder1) EntityIds(entityIds ...string) elBuilder2 {
	if len(entityIds) == 0 {
		panic("must pass at least one entityId to EntityIds()")
	} else {
		b.entityListener.entityIds = entityIds
	}
	return elBuilder2(b)
}

type elBuilder2 struct {
	entityListener EntityListener
}

func (b elBuilder2) Call(callback EntityListenerCallback) elBuilder3 {
	b.entityListener.callback = callback
	return elBuilder3(b)
}

type elBuilder3 struct {
	entityListener EntityListener
}

func (b elBuilder3) OnlyBetween(start string, end string) elBuilder3 {
	b.entityListener.betweenStart = start
	b.entityListener.betweenEnd = end
	return b
}

func (b elBuilder3) OnlyAfter(start string) elBuilder3 {
	b.entityListener.betweenStart = start
	return b
}

func (b elBuilder3) OnlyBefore(end string) elBuilder3 {
	b.entityListener.betweenEnd = end
	return b
}

func (b elBuilder3) FromState(s string) elBuilder3 {
	b.entityListener.fromState = s
	return b
}

func (b elBuilder3) ToState(s string) elBuilder3 {
	b.entityListener.toState = s
	return b
}

func (b elBuilder3) Duration(s types.DurationString) elBuilder3 {
	d := internal.ParseDuration(string(s))
	b.entityListener.delay = d
	return b
}

func (b elBuilder3) Throttle(s types.DurationString) elBuilder3 {
	d := internal.ParseDuration(string(s))
	b.entityListener.throttle = d
	return b
}

func (b elBuilder3) ExceptionDates(t time.Time, tl ...time.Time) elBuilder3 {
	b.entityListener.exceptionDates = append(tl, t)
	return b
}

func (b elBuilder3) ExceptionRange(start, end time.Time) elBuilder3 {
	b.entityListener.exceptionRanges = append(b.entityListener.exceptionRanges, types.TimeRange{Start: start, End: end})
	return b
}

func (b elBuilder3) RunOnStartup() elBuilder3 {
	b.entityListener.runOnStartup = true
	return b
}

// EnabledWhen enables this listener only when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the listener runs if {runOnNetworkError} is true.
func (b elBuilder3) EnabledWhen(entityId, state string, runOnNetworkError bool) elBuilder3 {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	b.entityListener.enabledEntities = append(b.entityListener.enabledEntities, i)
	return b
}

// DisabledWhen disables this listener when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the listener runs if {runOnNetworkError} is true.
func (b elBuilder3) DisabledWhen(entityId, state string, runOnNetworkError bool) elBuilder3 {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	b.entityListener.disabledEntities = append(b.entityListener.disabledEntities, i)
	return b
}

func (b elBuilder3) Build() EntityListener {
	return b.entityListener
}

func callEntityListeners(app *App, msgBytes []byte) {
	msg := stateChangedMsg{}
	_ = json.Unmarshal(msgBytes, &msg)
	data := msg.Event.Data
	eid := data.EntityID
	listeners, ok := app.entityListeners[eid]
	if !ok {
		// no listeners registered for this id
		return
	}

	// if new state is same as old state, don't call
	// event listener. I noticed this with iOS app location,
	// every time I refresh the app it triggers a device_tracker
	// entity listener.
	if msg.Event.Data.NewState.State == msg.Event.Data.OldState.State {
		return
	}

	for _, l := range listeners {
		// Check conditions
		if c := CheckWithinTimeRange(l.betweenStart, l.betweenEnd); c.fail {
			continue
		}
		if c := CheckStatesMatch(l.fromState, data.OldState.State); c.fail {
			continue
		}
		if c := CheckStatesMatch(l.toState, data.NewState.State); c.fail {
			if l.delayTimer != nil {
				l.delayTimer.Stop()
			}
			continue
		}
		if c := CheckThrottle(l.throttle, l.lastRan); c.fail {
			continue
		}
		if c := CheckExceptionDates(l.exceptionDates); c.fail {
			continue
		}
		if c := CheckExceptionRanges(l.exceptionRanges); c.fail {
			continue
		}
		if c := CheckEnabledEntity(app.state, l.enabledEntities); c.fail {
			continue
		}
		if c := CheckDisabledEntity(app.state, l.disabledEntities); c.fail {
			continue
		}

		entityData := EntityData{
			TriggerEntityId: eid,
			FromState:       data.OldState.State,
			FromAttributes:  data.OldState.Attributes,
			ToState:         data.NewState.State,
			ToAttributes:    data.NewState.Attributes,
			LastChanged:     data.OldState.LastChanged,
		}

		if l.delay != 0 {
			l := l
			l.delayTimer = time.AfterFunc(l.delay, func() {
				go l.callback(app.service, app.state, entityData)
				l.lastRan = carbon.Now()
			})
			continue
		}

		// run now if no delay set
		go l.callback(app.service, app.state, entityData)
		l.lastRan = carbon.Now()
	}
}
