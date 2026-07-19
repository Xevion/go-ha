package scheduling

import (
	"fmt"
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/dromara/carbon/v2"
)

type Trigger interface {
	// NextTime calculates the next occurrence of this trigger after the given time
	NextTime(now time.Time) *time.Time
}

// FixedTimeTrigger represents a trigger at a specific hour and minute each day
type FixedTimeTrigger struct {
	Hour   int // 0-23
	Minute int // 0-59
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
