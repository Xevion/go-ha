package scheduling

import (
	"fmt"
	"hash/fnv"
	"strings"
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

// Location is the observer position that sun triggers are computed against.
type Location struct {
	Latitude  float64
	Longitude float64
}

// SunTrigger represents a trigger based on sunrise or sunset with optional offset
type SunTrigger struct {
	latitude  float64        // latitude of the location
	longitude float64        // longitude of the location
	sunset    bool           // true for sunset, false for sunrise
	offset    *time.Duration // offset from sun event (can be negative)
}

func (t *FixedTimeTrigger) NextTime(now time.Time) *time.Time {
	next := carbon.NewCarbon(now).SetTimeMilli(t.Hour, t.Minute, 0, 0)

	// If the calculated time is before or equal to now, advance to the next day
	if !next.StdTime().After(now) {
		next = next.AddDay()
	}

	return internal.Ptr(next.StdTime().Local())
}

func (t *FixedTimeTrigger) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// Hash returns a stable hash value for the FixedTimeTrigger
func (t *FixedTimeTrigger) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "%d:%d", t.Hour, t.Minute)
	return h.Sum64()
}

// sunSearchDays bounds how far ahead NextTime looks. Above the polar circles
// the sun can stay up or down for months, and no schedule is served by scanning
// that far.
const sunSearchDays = 4

// NextTime returns the next time the sun will rise or set, with the offset
// applied. It scans forward day by day: today's event has usually passed by the
// time the scheduler asks, and the offset can push it either side of now.
func (t *SunTrigger) NextTime(now time.Time) *time.Time {
	for day := range sunSearchDays {
		d := now.AddDate(0, 0, day)

		var sun time.Time
		if t.sunset {
			_, sun = sunrise.SunriseSunset(t.latitude, t.longitude, d.Year(), d.Month(), d.Day())
		} else {
			sun, _ = sunrise.SunriseSunset(t.latitude, t.longitude, d.Year(), d.Month(), d.Day())
		}

		// The sun neither rises nor sets on this day at this latitude.
		if sun.IsZero() {
			continue
		}

		sun = sun.Local()
		if t.offset != nil && *t.offset != 0 {
			sun = sun.Add(*t.offset)
		}

		if sun.After(now) {
			return &sun
		}
	}

	return nil
}

func (t *SunTrigger) String() string {
	return fmt.Sprintf("%s at %.4f,%.4f", sunLabel(t.sunset, t.offset), t.latitude, t.longitude)
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

func (c *CompositeDailySchedule) String() string {
	parts := make([]string, 0, len(c.triggers))
	for _, trigger := range c.triggers {
		parts = append(parts, fmt.Sprint(trigger))
	}
	return strings.Join(parts, ", ")
}

// Hash returns a stable hash value for the CompositeDailySchedule
func (c *CompositeDailySchedule) Hash() uint64 {
	h := fnv.New64()
	for _, trigger := range c.triggers {
		fmt.Fprintf(h, "%d", trigger.Hash())
	}
	return h.Sum64()
}
