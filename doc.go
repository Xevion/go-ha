// Package ha writes Home Assistant automations in Go.
//
// An automation is four layers: a trigger decides when to consider running, a
// condition decides whether to go ahead, a policy decides what to do about
// overlapping runs, and an action does the work.
//
//	app.RegisterAutomations(
//		ha.NewAutomation("hall light").
//			On(ha.StateChanged("binary_sensor.hall_motion").To("on")).
//			When(ha.SunIsDown()).
//			Throttle(time.Minute).
//			Do(func(ctx context.Context, run ha.Run) error {
//				return run.Services.Light.TurnOn("light.hall")
//			}).
//			MustBuild(),
//	)
//
// # Triggers
//
// Triggers belong to one of two families. A [ScheduleTrigger] can say when it
// will next fire and is driven from a timing heap: [Daily], [Every], [Cron],
// [Sunrise], [Sunset], [Dawn], [Dusk] and [AtStartup]. An [EventTrigger]
// declares what it needs delivered and is driven from the event stream:
// [StateChanged] and [EventFired].
//
// One automation may hold a mix of both, which is what lets a single rule say
// "at sunset, or when the door opens". Event triggers declare their
// subscriptions rather than subscribing imperatively, which is what allows them
// to be replayed after a reconnect instead of being silently lost.
//
// Sun times come from Home Assistant's own sun.sun entity. It runs astral
// against the observer's latitude, longitude and elevation with a configurable
// solar depression, so computing them locally would disagree with the times on
// the user's own dashboard.
//
// # Conditions
//
// Conditions compose with [All], [Any] and [Not]. An error from a condition
// means undecided rather than false, and a definite answer from any branch
// settles the expression even when a sibling could not be evaluated. What
// happens when nothing settles it is the automation's choice, via
// [AutomationBuilder.OnConditionError].
//
// # Policy
//
// [Mode] mirrors Home Assistant's automation mode and applies to the automation
// as a whole. Throttle is counted per entity, so one automation watching many
// entities keeps a separate window for each.
//
// # State
//
// Entity state is cached locally, seeded from the REST API on every connection
// and maintained from the event stream. A condition therefore costs a map
// lookup rather than an HTTP round trip, and automations keep working through a
// disconnect.
//
// # Testing
//
// The hatest package runs an in-process Home Assistant, so automations can be
// exercised end to end and asserted on by the service calls they made.
package ha
