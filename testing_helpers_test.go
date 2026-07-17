package ha

import (
	"time"

	"github.com/Xevion/go-ha/internal"
)

func testClock() *internal.FakeClock {
	return internal.NewFakeClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
}
