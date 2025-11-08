package ha

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/scheduling"
	"github.com/Xevion/go-ha/types"
)

type IntervalCallback func(*Service, StateReader)

type Interval struct {
	// Drives when the interval fires. Built by Every, anchored by StartingAt.
	trigger *scheduling.IntervalTrigger
	// Any error raised while describing the interval, surfaced at registration.
	triggerErr error

	callback  IntervalCallback
	startTime types.TimeString
	endTime   types.TimeString

	dateFilter

	enabledEntities  []internal.EnabledDisabledInfo
	disabledEntities []internal.EnabledDisabledInfo
}

func (i Interval) Hash() string {
	var trigger uint64
	if i.trigger != nil {
		trigger = i.trigger.Hash()
	}
	return fmt.Sprint(trigger, i.startTime, i.endTime, i.callback, i.exceptionDates, i.exceptionRanges)
}

// Call
type intervalBuilder struct {
	interval Interval
}

// Every
type intervalBuilderCall struct {
	interval Interval
}

// Offset, ExceptionDates, ExceptionRange
type intervalBuilderEnd struct {
	interval Interval
}

func NewInterval() intervalBuilder {
	return intervalBuilder{
		Interval{
			startTime: "00:00",
			endTime:   "00:00",
		},
	}
}

func (i Interval) String() string {
	if i.trigger == nil {
		return fmt.Sprintf("Interval{ call %q }", internal.GetFunctionName(i.callback))
	}
	return fmt.Sprintf("Interval{ call %q %s%s%s }",
		internal.GetFunctionName(i.callback),
		i.trigger,
		formatStartOrEndString(i.startTime, true),
		formatStartOrEndString(i.endTime, false),
	)
}

func formatStartOrEndString(s types.TimeString, isStart bool) string {
	if s == "00:00" {
		return ""
	}
	if isStart {
		return fmt.Sprintf(" starting at %s", s)
	} else {
		return fmt.Sprintf(" ending at %s", s)
	}
}

func (ib intervalBuilder) Call(callback IntervalCallback) intervalBuilderCall {
	ib.interval.callback = callback
	return intervalBuilderCall(ib)
}

// Every takes a DurationString ("2h", "5m", etc.) to set the frequency of the interval.
func (ib intervalBuilderCall) Every(s types.DurationString) intervalBuilderEnd {
	d := internal.ParseDuration(string(s))
	ib.interval.trigger, ib.interval.triggerErr = scheduling.NewIntervalTrigger(d)
	return intervalBuilderEnd(ib)
}

// StartingAt takes a TimeString ("HH:MM") when this interval will start running for the day.
func (ib intervalBuilderEnd) StartingAt(s types.TimeString) intervalBuilderEnd {
	ib.interval.startTime = s
	if ib.interval.trigger != nil {
		ib.interval.trigger.WithEpoch(internal.ParseTime(internal.RealClock{}, string(s)).StdTime())
	}
	return ib
}

// EndingAt takes a TimeString ("HH:MM") when this interval will stop running for the day.
func (ib intervalBuilderEnd) EndingAt(s types.TimeString) intervalBuilderEnd {
	ib.interval.endTime = s
	return ib
}

func (ib intervalBuilderEnd) ExceptionDates(t time.Time, tl ...time.Time) intervalBuilderEnd {
	ib.interval.addExceptions(t, tl...)
	return ib
}

func (ib intervalBuilderEnd) ExceptionRange(start, end time.Time) intervalBuilderEnd {
	ib.interval.addRange(start, end)
	return ib
}

// Enable this interval only when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the interval runs if {runOnNetworkError} is true.
func (ib intervalBuilderEnd) EnabledWhen(entityId, state string, runOnNetworkError bool) intervalBuilderEnd {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	ib.interval.enabledEntities = append(ib.interval.enabledEntities, i)
	return ib
}

// Disable this interval when the current state of {entityId} matches {state}.
// If there is a network error while retrieving state, the interval runs if {runOnNetworkError} is true.
func (ib intervalBuilderEnd) DisabledWhen(entityId, state string, runOnNetworkError bool) intervalBuilderEnd {
	if entityId == "" {
		panic(fmt.Sprintf("entityId is empty in EnabledWhen entityId='%s' state='%s'", entityId, state))
	}
	i := internal.EnabledDisabledInfo{
		Entity:     entityId,
		State:      state,
		RunOnError: runOnNetworkError,
	}
	ib.interval.disabledEntities = append(ib.interval.disabledEntities, i)
	return ib
}

func (sb intervalBuilderEnd) Build() Interval {
	return sb.interval
}

// app.Start() functions
func runIntervals(a *App) {
	if a.intervals.len() == 0 {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			slog.Info("Intervals goroutine shutting down")
			return
		default:
		}

		// Run callbacks for everything already due, in case slots overlapped.
		a.intervals.runDue(a.clock.Now())

		entry := a.intervals.peek()
		if entry == nil {
			slog.Info("No intervals left to run")
			return
		}

		// Use context-aware sleep
		select {
		case <-time.After(time.Until(entry.fireAt)):
			// Time elapsed, the next pass runs it
		case <-a.ctx.Done():
			slog.Info("Intervals goroutine shutting down")
			return
		}
	}
}

func (i Interval) maybeRunCallback(a *App) {
	if c := CheckStartEndTime(a.clock, i.startTime, true); c.fail {
		return
	}
	if c := CheckStartEndTime(a.clock, i.endTime, false); c.fail {
		return
	}
	if c := CheckExceptionDates(a.clock, i.exceptionDates); c.fail {
		return
	}
	if c := CheckExceptionRanges(a.clock, i.exceptionRanges); c.fail {
		return
	}
	if c := CheckEnabledEntity(a.state, i.enabledEntities); c.fail {
		return
	}
	if c := CheckDisabledEntity(a.state, i.disabledEntities); c.fail {
		return
	}
	go i.callback(a.service, a.state)
}
