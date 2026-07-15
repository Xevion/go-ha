package ha

import (
	"context"
	"errors"
	"fmt"
	"time"
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

// on returns this time of day on the date of ref, in ref's location.
func (c ClockTime) on(ref time.Time) time.Time {
	y, m, d := ref.Date()
	return time.Date(y, m, d, c.hour, c.minute, 0, 0, ref.Location())
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

// BeforeTime holds from midnight until the given time.
func BeforeTime(t ClockTime) Condition {
	return timeBetweenCondition{start: TimeOfDay(0, 0), end: t}
}

func (c timeBetweenCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	now := ec.Clock.Now()
	start := c.start.on(now)
	end := c.end.on(now)

	// An end at or before the start means the range runs through midnight, so
	// the two halves are on either side of it rather than between them.
	if !end.After(start) {
		return !now.Before(start) || now.Before(end), nil
	}
	return !now.Before(start) && now.Before(end), nil
}

func (c timeBetweenCondition) validate() error {
	return errors.Join(c.start.err, c.end.err)
}

func (c timeBetweenCondition) String() string {
	return fmt.Sprintf("between %s and %s", c.start, c.end)
}
