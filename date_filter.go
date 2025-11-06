package ha

import (
	"time"

	"github.com/Xevion/go-ha/types"
)

// dateFilter holds the date restrictions every automation kind shares. Each
// builder embeds one so the append semantics live in a single place.
type dateFilter struct {
	// Dates the automation should NOT run on
	exceptionDates []time.Time
	// Dates the automation is ONLY allowed to run on, if any are set
	allowlistDates []time.Time
	// Datetime ranges the automation should NOT run within
	exceptionRanges []types.TimeRange
}

func (f *dateFilter) addExceptions(t time.Time, tl ...time.Time) {
	f.exceptionDates = append(append(f.exceptionDates, t), tl...)
}

func (f *dateFilter) addAllowlist(t time.Time, tl ...time.Time) {
	f.allowlistDates = append(append(f.allowlistDates, t), tl...)
}

func (f *dateFilter) addRange(start, end time.Time) {
	f.exceptionRanges = append(f.exceptionRanges, types.TimeRange{Start: start, End: end})
}
