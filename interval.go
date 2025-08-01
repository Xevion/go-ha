package gomeassistant

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/internal/parse"
	"github.com/Xevion/go-ha/types"
)

type IntervalCallback func(*Service, State)

type Interval struct {
	frequency   time.Duration
	callback    IntervalCallback
	startTime   types.TimeString
	endTime     types.TimeString
	nextRunTime time.Time

	exceptionDates  []time.Time
	exceptionRanges []types.TimeRange

	enabledEntities  []internal.EnabledDisabledInfo
	disabledEntities []internal.EnabledDisabledInfo
}

func (i Interval) Hash() string {
	return fmt.Sprint(i.startTime, i.endTime, i.frequency, i.callback, i.exceptionDates, i.exceptionRanges)
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
			frequency: 0,
			startTime: "00:00",
			endTime:   "00:00",
		},
	}
}

func (i Interval) String() string {
	return fmt.Sprintf("Interval{ call %q every %s%s%s }",
		internal.GetFunctionName(i.callback),
		i.frequency,
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

// Takes a DurationString ("2h", "5m", etc) to set the frequency of the interval.
func (ib intervalBuilderCall) Every(s types.DurationString) intervalBuilderEnd {
	d := parse.ParseDuration(string(s))
	ib.interval.frequency = d
	return intervalBuilderEnd(ib)
}

// Takes a TimeString ("HH:MM") when this interval will start running for the day.
func (ib intervalBuilderEnd) StartingAt(s types.TimeString) intervalBuilderEnd {
	ib.interval.startTime = s
	return ib
}

// Takes a TimeString ("HH:MM") when this interval will stop running for the day.
func (ib intervalBuilderEnd) EndingAt(s types.TimeString) intervalBuilderEnd {
	ib.interval.endTime = s
	return ib
}

func (ib intervalBuilderEnd) ExceptionDates(t time.Time, tl ...time.Time) intervalBuilderEnd {
	ib.interval.exceptionDates = append(tl, t)
	return ib
}

func (ib intervalBuilderEnd) ExceptionRange(start, end time.Time) intervalBuilderEnd {
	ib.interval.exceptionRanges = append(
		ib.interval.exceptionRanges,
		types.TimeRange{Start: start, End: end},
	)
	return ib
}

/*
Enable this interval only when the current state of {entityId} matches {state}.
If there is a network error while retrieving state, the interval runs if {runOnNetworkError} is true.
*/
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

/*
Disable this interval when the current state of {entityId} matches {state}.
If there is a network error while retrieving state, the interval runs if {runOnNetworkError} is true.
*/
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
	if a.intervals.Len() == 0 {
		return
	}

	for {
		select {
		case <-a.ctx.Done():
			slog.Info("Intervals goroutine shutting down")
			return
		default:
		}

		i := popInterval(a)

		// run callback for all intervals before now in case they overlap
		for i.nextRunTime.Before(time.Now()) {
			i.maybeRunCallback(a)
			requeueInterval(a, i)

			i = popInterval(a)
		}

		// Use context-aware sleep
		select {
		case <-time.After(time.Until(i.nextRunTime)):
			// Time elapsed, continue
		case <-a.ctx.Done():
			slog.Info("Intervals goroutine shutting down")
			return
		}

		i.maybeRunCallback(a)
		requeueInterval(a, i)
	}
}

func (i Interval) maybeRunCallback(a *App) {
	if c := CheckStartEndTime(i.startTime /* isStart = */, true); c.fail {
		return
	}
	if c := CheckStartEndTime(i.endTime /* isStart = */, false); c.fail {
		return
	}
	if c := CheckExceptionDates(i.exceptionDates); c.fail {
		return
	}
	if c := CheckExceptionRanges(i.exceptionRanges); c.fail {
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

func popInterval(a *App) Interval {
	i, _ := a.intervals.Get(1)
	return i[0].(Item).Value.(Interval)
}

func requeueInterval(a *App, i Interval) {
	i.nextRunTime = i.nextRunTime.Add(i.frequency)

	a.intervals.Put(Item{
		Value:    i,
		Priority: float64(i.nextRunTime.Unix()),
	})
}
