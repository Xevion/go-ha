package ha

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type onDatesCondition struct{ dates []time.Time }

// OnDates holds on any of the given calendar days, whatever the time. Wrap it
// in Not for the holidays-excepted case.
func OnDates(dates ...time.Time) Condition {
	return onDatesCondition{dates: dates}
}

func (c onDatesCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	now := ec.Clock.Now()
	for _, d := range c.dates {
		if sameDate(d, now) {
			return true, nil
		}
	}
	return false, nil
}

func (c onDatesCondition) validate() error {
	if len(c.dates) == 0 {
		return fmt.Errorf("%w: OnDates needs at least one date", ErrInvalidArgs)
	}
	return nil
}

func (c onDatesCondition) String() string {
	return fmt.Sprintf("on %d date(s)", len(c.dates))
}

type inDateRangeCondition struct{ start, end time.Time }

// InDateRange holds from start until end, start included and end excluded.
func InDateRange(start, end time.Time) Condition {
	return inDateRangeCondition{start: start, end: end}
}

func (c inDateRangeCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	now := ec.Clock.Now()
	return !now.Before(c.start) && now.Before(c.end), nil
}

func (c inDateRangeCondition) validate() error {
	if !c.end.After(c.start) {
		return fmt.Errorf("date range ends at %s, which is not after its start %s", c.end, c.start)
	}
	return nil
}

func (c inDateRangeCondition) String() string {
	return fmt.Sprintf("between %s and %s", c.start, c.end)
}

type onWeekdaysCondition struct{ days []time.Weekday }

// OnWeekdays holds on the given days of the week.
func OnWeekdays(days ...time.Weekday) Condition {
	return onWeekdaysCondition{days: days}
}

func (c onWeekdaysCondition) Eval(_ context.Context, ec EvalContext) (bool, error) {
	return slices.Contains(c.days, ec.Clock.Now().Weekday()), nil
}

func (c onWeekdaysCondition) validate() error {
	if len(c.days) == 0 {
		return fmt.Errorf("%w: OnWeekdays needs at least one day", ErrInvalidArgs)
	}
	return nil
}

func (c onWeekdaysCondition) String() string {
	return fmt.Sprintf("on %v", c.days)
}
