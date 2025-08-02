package scheduling

import (
	"fmt"
	"hash/fnv"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/dromara/carbon/v2"
	"github.com/nathan-osman/go-sunrise"
)

type Trigger interface {
	// NextTime calculates the next occurrence of this trigger after the given time
	NextTime(now time.Time) *time.Time
	Hash() uint64
}

// FixedTimeTrigger represents a trigger at a specific hour and minute each day
type FixedTimeTrigger struct {
	Hour   int // 0-23
	Minute int // 0-59
}

// SunTrigger represents a trigger based on sunrise or sunset with optional offset
type SunTrigger struct {
	latitude  float64        // latitude of the location
	longitude float64        // longitude of the location
	sunset    bool           // true for sunset, false for sunrise
	offset    *time.Duration // offset from sun event (can be negative)
}

func (t *FixedTimeTrigger) NextTime(now time.Time) *time.Time {
	next := carbon.NewCarbon(now).SetHour(t.Hour).SetMinute(t.Minute)

	// If the calculated time is before or equal to now, advance to the next day
	if !next.StdTime().After(now) {
		next = next.AddDay()
	}

	return internal.Ptr(next.StdTime().Local())
}

// Hash returns a stable hash value for the FixedTimeTrigger
func (t *FixedTimeTrigger) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "%d:%d", t.Hour, t.Minute)
	return h.Sum64()
}

// NextTime returns the next time the sun will rise or set. If an offset is provided, it will be added to the calculated time.
func (t *SunTrigger) NextTime(now time.Time) *time.Time {
	var sun time.Time

	if t.sunset {
		_, sun = sunrise.SunriseSunset(t.latitude, t.longitude, now.Year(), now.Month(), now.Day())
	} else {
		sun, _ = sunrise.SunriseSunset(t.latitude, t.longitude, now.Year(), now.Month(), now.Day())
	}

	// In the case that the sun does not rise or set on the given day, return nil
	if sun.IsZero() {
		return nil
	}

	sun = sun.Local() // Convert to local time
	if t.offset != nil && *t.offset != 0 {
		sun = sun.Add(*t.offset) // Add the offset if provided and not zero
	}

	return &sun
}

// Hash returns a stable hash value for the SunTrigger
func (t *SunTrigger) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "%f:%f:%t", t.latitude, t.longitude, t.sunset)
	if t.offset != nil {
		fmt.Fprintf(h, ":%d", t.offset.Nanoseconds())
	}
	return h.Sum64()
}

// CompositeDailySchedule combines multiple triggers into a single daily schedule.
type CompositeDailySchedule struct {
	triggers []Trigger
}

// NextTime returns the next time the first viable trigger will run.
func (c *CompositeDailySchedule) NextTime(now time.Time) *time.Time {
	best := c.triggers[0].NextTime(now)

	for _, trigger := range c.triggers[1:] {
		potential := trigger.NextTime(now)
		if potential != nil && (best == nil || potential.Before(*best)) {
			best = potential
		}
	}

	return best
}

// Hash returns a stable hash value for the CompositeDailySchedule
func (c *CompositeDailySchedule) Hash() uint64 {
	h := fnv.New64()
	for _, trigger := range c.triggers {
		fmt.Fprintf(h, "%d", trigger.Hash())
	}
	return h.Sum64()
}
