package types

import "time"

// DurationString represents a duration, such as "2s" or "24h".
// See https://pkg.go.dev/time#ParseDuration for all valid time units.
type DurationString string

// TimeString is a 24-hr format time "HH:MM" such as "07:30".
type TimeString string

// TimeRange represents a time range with start and end times.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// Item represents a priority queue item with a value and priority.
type Item struct {
	Value    interface{}
	Priority float64
}
