package types

import "time"

// Clock is the time source an App reads. Supplying one makes schedules,
// throttles and For durations testable without waiting on the wall clock.
type Clock interface {
	Now() time.Time
}
