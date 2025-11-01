// Package ha provides a Go library for creating Home Assistant automations
// and schedules. This file contains the scheduling system that allows you to create
// daily schedules with various conditions and callbacks.
package ha

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/Xevion/go-ha/types"
)

// ScheduleCallback is a function type that gets called when a schedule triggers.
// It receives the service instance and current state as parameters.
type ScheduleCallback func(*Service, StateReader)

// DailySchedule represents a recurring daily schedule with various conditions.
// It can be configured to run at specific times, sunrise/sunset, or based on
// entity states and date restrictions.
type DailySchedule struct {
	// Describes when this schedule fires. Resolved into a Trigger during
	// registration, once the home zone coordinates are known.
	spec scheduling.Spec
	// Any error raised while describing the schedule, surfaced at registration.
	specErr error

	// Function to call when the schedule triggers
	callback ScheduleCallback

	// Dates when this schedule should NOT run
	exceptionDates []time.Time
	// Dates when this schedule is ONLY allowed to run (if empty, runs on all dates)
	allowlistDates []time.Time

	// Entities that must be in specific states for this schedule to run
	enabledEntities []internal.EnabledDisabledInfo
	// Entities that must NOT be in specific states for this schedule to run
	disabledEntities []internal.EnabledDisabledInfo
}

// Hash returns a unique string identifier for this schedule based on when it
// fires and the callback function.
func (s DailySchedule) Hash() string {
	if s.spec == nil {
		return fmt.Sprint(s.callback)
	}
	return fmt.Sprint(s.spec.Hash(), s.callback)
}

// scheduleBuilder is used in the fluent API to build schedules step by step.
type scheduleBuilder struct {
	schedule DailySchedule
}

// scheduleBuilderCall represents the state after setting the callback function.
type scheduleBuilderCall struct {
	schedule DailySchedule
}

// scheduleBuilderEnd represents the final state where time and conditions are set.
type scheduleBuilderEnd struct {
	schedule DailySchedule
}

// NewDailySchedule creates a new schedule builder with default values.
// Use the fluent API to configure the schedule:
//
//	NewDailySchedule().Call(myFunction).At("15:30").Build()
func NewDailySchedule() scheduleBuilder {
	return scheduleBuilder{DailySchedule{}}
}

// String returns a human-readable representation of the schedule.
func (s DailySchedule) String() string {
	return fmt.Sprintf("Schedule{ call %q }", internal.GetFunctionName(s.callback))
}

// Call sets the callback function that will be executed when the schedule triggers.
// This is the first step in the fluent API chain.
func (sb scheduleBuilder) Call(callback ScheduleCallback) scheduleBuilderCall {
	sb.schedule.callback = callback
	return scheduleBuilderCall(sb)
}

// At sets the schedule to run at a specific time in 24-hour format.
// Examples: "15:30", "09:00", "23:45"
func (sb scheduleBuilderCall) At(s string) scheduleBuilderEnd {
	t := internal.ParseTime(internal.RealClock{}, s)
	sb.schedule.spec, sb.schedule.specErr = scheduling.NewSchedule().OnFixedTime(t.Hour(), t.Minute()).Build()
	return scheduleBuilderEnd(sb)
}

// Sunrise configures the schedule to run at sunrise with an optional offset.
// The offset parameter is a duration string (e.g., "-30m", "+1h", "-1.5h").
// Only the first offset, if provided, is considered.
// Examples:
//   - Sunrise() - runs at sunrise
//   - Sunrise("-30m") - runs 30 minutes before sunrise
//   - Sunrise("+1h") - runs 1 hour after sunrise
func (sb scheduleBuilderCall) Sunrise(offset ...types.DurationString) scheduleBuilderEnd {
	sb.schedule.spec, sb.schedule.specErr = scheduling.NewSchedule().OnSunrise(offset...).Build()
	return scheduleBuilderEnd(sb)
}

// Sunset configures the schedule to run at sunset with an optional offset.
// The offset parameter is a duration string (e.g., "-30m", "+1h", "-1.5h").
// Only the first offset, if provided, is considered.
// Examples:
//   - Sunset() - runs at sunset
//   - Sunset("-30m") - runs 30 minutes before sunset
//   - Sunset("+1h") - runs 1 hour after sunset
func (sb scheduleBuilderCall) Sunset(offset ...types.DurationString) scheduleBuilderEnd {
	sb.schedule.spec, sb.schedule.specErr = scheduling.NewSchedule().OnSunset(offset...).Build()
	return scheduleBuilderEnd(sb)
}

// ExceptionDates adds dates when this schedule should NOT run.
// You can pass multiple dates: ExceptionDates(date1, date2, date3)
func (sb scheduleBuilderEnd) ExceptionDates(t time.Time, tl ...time.Time) scheduleBuilderEnd {
	sb.schedule.exceptionDates = append(tl, t)
	return sb
}

// OnlyOnDates restricts the schedule to run ONLY on the specified dates.
// If no dates are specified, the schedule runs on all dates.
// You can pass multiple dates: OnlyOnDates(date1, date2, date3)
func (sb scheduleBuilderEnd) OnlyOnDates(t time.Time, tl ...time.Time) scheduleBuilderEnd {
	sb.schedule.allowlistDates = append(tl, t)
	return sb
}

// EnabledWhen makes this schedule only run when the specified entity is in the given state.
// If there's a network error while checking the entity state, the schedule runs
// only if runOnNetworkError is true.
// Examples:
//   - EnabledWhen("light.living_room", "on", true) - only run when light is on
//   - EnabledWhen("sensor.motion", "detected", false) - only run when motion detected, fail on network error
func (sb scheduleBuilderEnd) EnabledWhen(entityId, state string, runOnNetworkError bool) scheduleBuilderEnd {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	sb.schedule.enabledEntities = append(sb.schedule.enabledEntities, i)
	return sb
}

// DisabledWhen prevents this schedule from running when the specified entity is in the given state.
// If there's a network error while checking the entity state, the schedule runs only if runOnNetworkError is true.
// Examples:
//   - DisabledWhen("light.living_room", "off", true) - don't run when light is off
//   - DisabledWhen("sensor.motion", "detected", false) - don't run when motion detected, fail on network error
func (sb scheduleBuilderEnd) DisabledWhen(entityId, state string, runOnNetworkError bool) scheduleBuilderEnd {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	sb.schedule.disabledEntities = append(sb.schedule.disabledEntities, i)
	return sb
}

// Build finalizes the schedule configuration and returns the DailySchedule.
// This is the final step in the fluent API chain.
func (sb scheduleBuilderEnd) Build() DailySchedule {
	return sb.schedule
}

// runSchedules is the main goroutine that manages all schedules.
// It continuously processes schedules, running them when their time comes
// and requeuing them for the next day.
func runSchedules(a *App) {
	if a.schedules.len() == 0 {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			slog.Info("Schedules goroutine shutting down")
			return
		default:
		}

		entry := a.schedules.pop()
		if entry == nil {
			slog.Info("No schedules left to run")
			return
		}

		// Run callback for all schedules that are overdue in case they overlap
		for entry.fireAt.Before(a.clock.Now()) {
			entry.run()
			a.schedules.requeue(entry)

			entry = a.schedules.pop()
			if entry == nil {
				slog.Info("No schedules left to run")
				return
			}
		}

		slog.Info("Next schedule", "start_time", entry.fireAt)

		// Wait until the next schedule time or context cancellation
		select {
		case <-time.After(time.Until(entry.fireAt)):
			// Time elapsed, continue
		case <-a.ctx.Done():
			slog.Info("Schedules goroutine shutting down")
			return
		}

		entry.run()
		a.schedules.requeue(entry)
	}
}

// maybeRunCallback checks all conditions and runs the callback if they're all met.
// Conditions checked:
// 1. Exception dates (schedule should not run on these dates)
// 2. Allowlist dates (schedule should only run on these dates)
// 3. Enabled entities (required entity states)
// 4. Disabled entities (forbidden entity states)
// The callback runs in a goroutine to avoid blocking the scheduler.
func (s DailySchedule) maybeRunCallback(a *App) {
	if c := CheckExceptionDates(a.clock, s.exceptionDates); c.fail {
		return
	}
	if c := CheckAllowlistDates(a.clock, s.allowlistDates); c.fail {
		return
	}
	if c := CheckEnabledEntity(a.state, s.enabledEntities); c.fail {
		return
	}
	if c := CheckDisabledEntity(a.state, s.disabledEntities); c.fail {
		return
	}
	go s.callback(a.service, a.state)
}
