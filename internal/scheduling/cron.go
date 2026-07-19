package scheduling

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// CronTrigger represents a trigger based on a cron expression.
type CronTrigger struct {
	expression string
	schedule   cron.Schedule
}

// NewCronTrigger creates a new CronTrigger from a cron expression.
func NewCronTrigger(expression string) (*CronTrigger, error) {
	// Use the standard cron parser
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	return &CronTrigger{
		expression: expression,
		schedule:   schedule,
	}, nil
}

// NextTime calculates the next occurrence of this cron trigger after the given time.
func (t *CronTrigger) NextTime(now time.Time) *time.Time {
	next := t.schedule.Next(now)
	return &next
}

func (t *CronTrigger) String() string {
	return "cron(" + t.expression + ")"
}
