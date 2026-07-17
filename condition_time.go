package ha

import (
	"context"
	"errors"
	"fmt"
)

// ErrInvalidTimeOfDay reports an hour or minute outside a real clock face.
var ErrInvalidTimeOfDay = errors.New("invalid time of day")

// ClockTime is a wall-clock time, independent of any date. Build one with
// TimeOfDay.
type ClockTime struct {
	hour   int
	minute int
	err    error
}

// TimeOfDay names a time on the clock face, replacing strings like "19:00".
// An out-of-range value is reported when the automation holding it is built.
func TimeOfDay(hour, minute int) ClockTime {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ClockTime{err: fmt.Errorf("%w: %d:%d", ErrInvalidTimeOfDay, hour, minute)}
	}
	return ClockTime{hour: hour, minute: minute}
}

func (c ClockTime) String() string {
	return fmt.Sprintf("%02d:%02d", c.hour, c.minute)
}

// minuteOfDay is this time as minutes since midnight.
func (c ClockTime) minuteOfDay() int {
	return c.hour*60 + c.minute
}

type timeBetweenCondition struct {
	start ClockTime
	end   ClockTime
}

// TimeBetween holds from start until end, start included and end excluded. A
// range whose end is before its start crosses midnight.
func TimeBetween(start, end ClockTime) Condition {
	return timeBetweenCondition{start: start, end: end}
}

// AfterTime holds from the given time until midnight.
func AfterTime(t ClockTime) Condition {
	return timeBetweenCondition{start: t, end: TimeOfDay(0, 0)}
}

// BeforeTime holds from midnight until the given time. Nothing is before
// midnight, so BeforeTime(TimeOfDay(0, 0)) never holds.
func BeforeTime(t ClockTime) Condition {
	if t.err == nil && t.minuteOfDay() == 0 {
		return neverCondition{}
	}
	return timeBetweenCondition{start: TimeOfDay(0, 0), end: t}
}

// neverCondition is an empty window. Left to the start/end comparison it would
// read as a range wrapping the whole day, which is its exact opposite.
type neverCondition struct{}

func (neverCondition) Eval(context.Context, EvalContext) (bool, error) { return false, nil }

func (neverCondition) String() string { return "never" }

// Eval compares wall-clock minutes rather than building absolute instants.
// A daylight saving jump deletes an hour from the local clock: asking Go for a
// time inside the gap silently yields one an hour earlier, which can order the
// end of a range before its start and turn a half hour window into almost the
// whole day. Reading the clock face directly is also what the range means.
func (c timeBetweenCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	now := ec.Clock.Now()
	current := now.Hour()*60 + now.Minute()
	start, end := c.start.minuteOfDay(), c.end.minuteOfDay()

	// An end at or before the start means the range runs through midnight, so
	// the two halves are on either side of it rather than between them.
	if end <= start {
		return current >= start || current < end, nil
	}
	return current >= start && current < end, nil
}

func (c timeBetweenCondition) validate() error {
	return errors.Join(c.start.err, c.end.err)
}

func (c timeBetweenCondition) String() string {
	return fmt.Sprintf("between %s and %s", c.start, c.end)
}
