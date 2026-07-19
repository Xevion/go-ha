package scheduling

import (
	"fmt"
	"strings"
	"time"
)

// IntervalTrigger represents a trigger that fires at a sequence of intervals.
type IntervalTrigger struct {
	intervals     []time.Duration
	epoch         time.Time
	totalDuration time.Duration
}

// NewIntervalTrigger creates a new IntervalTrigger from one or more durations.
// An error is returned if no intervals are provided or if any interval is not positive.
// The epoch is the reference point for all interval calculations.
// The duration between each time alternates between each interval (or, if there is only one interval, it is the interval).
// For example, if the intervals are [1h, 2h, 3h], the first time will be at epoch + 1h, the second time will be at
// epoch + 1h + 2h, the third time will be at epoch + 1h + 2h + 3h, and so on.
func NewIntervalTrigger(interval time.Duration, additional ...time.Duration) (*IntervalTrigger, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("intervals must be positive")
	}
	totalDuration := interval
	for _, d := range additional {
		if d <= 0 {
			return nil, fmt.Errorf("intervals must be positive")
		}
		totalDuration += d
	}

	return &IntervalTrigger{
		intervals:     append([]time.Duration{interval}, additional...),
		epoch:         time.Time{}, // default epoch is zero time
		totalDuration: totalDuration,
	}, nil
}

// WithEpoch returns a copy of the trigger anchored to epoch, the reference point
// for all interval calculations. It copies rather than mutates so that two
// automations built from one builder chain cannot end up sharing an epoch.
func (t *IntervalTrigger) WithEpoch(epoch time.Time) *IntervalTrigger {
	anchored := *t
	anchored.epoch = epoch
	return &anchored
}

// NextTime calculates the next occurrence of this interval trigger after the given time.
func (t *IntervalTrigger) NextTime(now time.Time) *time.Time {
	if t.totalDuration == 0 {
		return nil
	}

	// Cycle arithmetic is over absolute durations, so the epoch's location only
	// decides how the emitted instants report themselves. Default to local, to
	// match the fixed time and sun triggers.
	epoch := t.epoch
	if epoch.IsZero() {
		epoch = time.Unix(0, 0).Local()
	}

	// If the current time is before the epoch, the next time is the first one in the cycle.
	if now.Before(epoch) {
		next := epoch.Add(t.intervals[0])
		return &next
	}

	cyclesSinceEpoch := now.Sub(epoch) / t.totalDuration
	currentCycleStart := epoch.Add(time.Duration(cyclesSinceEpoch) * t.totalDuration)

	// Cycle through the offsets until the next time is found
	cycle := currentCycleStart
	for i := 0; i < len(t.intervals); i++ {
		cycle = cycle.Add(t.intervals[i])
		if cycle.After(now) {
			return &cycle
		}
	}

	// If we've reached here, it means we're at the end of a cycle.
	// The next time will be the first interval of the next cycle.
	nextCycleStart := currentCycleStart.Add(t.totalDuration)
	next := nextCycleStart.Add(t.intervals[0])
	return &next
}

func (t *IntervalTrigger) String() string {
	parts := make([]string, 0, len(t.intervals))
	for _, d := range t.intervals {
		parts = append(parts, d.String())
	}
	return "every " + strings.Join(parts, " then ")
}
