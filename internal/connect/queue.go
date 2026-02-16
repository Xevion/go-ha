package connect

import (
	"log/slog"
	"time"
)

// dropReportInterval is the shortest gap between two overflow warnings.
const dropReportInterval = 10 * time.Second

// dropReporter rate limits the warning emitted when events are shed, so a
// sustained overload produces one line every dropReportInterval carrying a
// count, rather than one line per dropped message.
//
// It needs no synchronisation: only the reader goroutine ever touches it, and
// each connection gets its own.
type dropReporter struct {
	since int
	last  time.Time
}

// record notes a dropped event and logs if enough time has passed.
func (r *dropReporter) record(queued int) {
	r.since++

	now := time.Now()
	if !r.last.IsZero() && now.Sub(r.last) < dropReportInterval {
		return
	}
	r.last = now

	slog.Warn("Event queue full, shedding events",
		"dropped", r.since,
		"queued", queued,
	)
	r.since = 0
}
