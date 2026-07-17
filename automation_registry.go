package ha

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/Xevion/go-ha/internal/connect"
)

// binding pairs an automation with the one trigger of its several that a given
// event type can fire.
type binding struct {
	automation Automation
	trigger    EventTrigger
}

// schedulerAdapter presents a public ScheduleTrigger to the internal scheduler,
// which reports absence with a nil pointer rather than a bool.
type schedulerAdapter struct {
	trigger ScheduleTrigger
}

func (a schedulerAdapter) NextTime(now time.Time) *time.Time {
	next, ok := a.trigger.NextTime(now)
	if !ok {
		return nil
	}
	return &next
}

func (a schedulerAdapter) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "%v", a.trigger)
	return h.Sum64()
}

func (a schedulerAdapter) String() string { return fmt.Sprint(a.trigger) }

// RegisterAutomations wires automations to their triggers. Schedule triggers go
// onto the timing heap and event triggers onto the dispatch map, so an
// automation holding both is driven from both.
//
// Every automation is registered that can be; the error reports the rest
// together rather than stopping at the first.
func (app *App) RegisterAutomations(automations ...Automation) error {
	var errs []error

	for _, a := range automations {
		if a.runtime == nil {
			errs = append(errs, fmt.Errorf("%w %q: not built, call Build or MustBuild",
				ErrInvalidAutomation, a.name))
			continue
		}

		// Build has no App to read a clock from, so it starts on the real one.
		// Registration is where the automation joins an app, and its throttle
		// has to measure against the same clock its conditions read.
		a.runtime.withClock(app.clock)

		app.listenersMu.Lock()
		app.runners[a.runtime] = struct{}{}
		app.listenersMu.Unlock()

		for _, t := range a.triggers {
			schedule, isSchedule := t.(ScheduleTrigger)
			event, isEvent := t.(EventTrigger)

			switch {
			case isSchedule && isEvent:
				// A type switch would silently pick one and drop the other
				// half of the trigger, which is worse than refusing it.
				errs = append(errs, fmt.Errorf("%w %q: trigger %T implements both trigger families, which is ambiguous",
					ErrInvalidAutomation, a.name, t))
			case isSchedule:
				app.scheduleAutomation(a, schedule)
			case isEvent:
				if err := app.subscribeAutomation(a, event); err != nil {
					errs = append(errs, err)
				}
			default:
				errs = append(errs, fmt.Errorf("%w %q: trigger %T is neither a schedule nor an event trigger",
					ErrInvalidAutomation, a.name, t))
			}
		}
	}

	return errors.Join(errs...)
}

func (app *App) scheduleAutomation(a Automation, trig ScheduleTrigger) {
	app.schedules.add(schedulerAdapter{trigger: trig}, func() {
		ec := EvalContext{Clock: app.clock, State: app.state}
		deps := Run{Services: app.service, State: app.state, Trigger: trig}

		// Schedules key on the empty string: there is no entity involved, so
		// one automation gets one slot.
		a.fire(app.ctx, ec, deps, "")
	})
}

func (app *App) subscribeAutomation(a Automation, trig EventTrigger) error {
	var fresh []string

	app.listenersMu.Lock()
	for _, sub := range trig.Subscriptions() {
		if _, seen := app.automations[sub.EventType]; !seen {
			fresh = append(fresh, sub.EventType)
		}
		app.automations[sub.EventType] = append(app.automations[sub.EventType],
			binding{automation: a, trigger: trig})
	}
	app.listenersMu.Unlock()

	// Subscribing only after the map is published and unlocked. Home Assistant
	// delivers as soon as the request lands, on a worker goroutine that reads
	// the very map being written here.
	var errs []error
	for _, eventType := range fresh {
		// state_changed is subscribed at construction to feed the cache, and
		// its dispatch already routes here.
		if eventType == eventStateChanged {
			continue
		}
		if err := app.client.Subscribe(
			connect.Subscription{EventType: eventType},
			app.onEvent,
		); err != nil {
			errs = append(errs, fmt.Errorf("subscribing to %s: %w", eventType, err))
		}
	}

	return errors.Join(errs...)
}

// dispatchEvent runs every automation whose trigger matches the event.
func (app *App) dispatchEvent(raw []byte) {
	ev := parseEvent(raw)
	if ev.Type == "" {
		return
	}

	app.listenersMu.RLock()
	bindings := app.automations[ev.Type]
	app.listenersMu.RUnlock()
	if len(bindings) == 0 {
		return
	}

	ec := EvalContext{Clock: app.clock, State: app.state, Event: ev}

	for _, b := range bindings {
		if !b.trigger.Matches(ev) {
			continue
		}

		deps := Run{Services: app.service, State: app.state, Event: ev, Trigger: b.trigger}

		// Keyed by entity, so one automation watching many entities keeps a
		// separate throttle window and run slot for each.
		if !b.automation.fire(app.ctx, ec, deps, ev.EntityID) {
			slog.Debug("Automation did not run", "automation", b.automation.name, "entity", ev.EntityID)
		}
	}
}
