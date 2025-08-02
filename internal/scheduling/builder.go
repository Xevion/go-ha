package scheduling

import (
	"fmt"
	"time"

	"github.com/Xevion/go-ha/types"
)

type DailyScheduleBuilder struct {
	errors   []error
	hashes   map[uint64]bool
	triggers []Trigger
}

func NewSchedule() *DailyScheduleBuilder {
	return &DailyScheduleBuilder{
		hashes: make(map[uint64]bool),
	}
}

// tryAddTrigger adds a trigger to the builder if it is not already present.
// If the trigger is already present, an error will be added to the builder's errors.
// It will return the builder for chaining.
func (b *DailyScheduleBuilder) tryAddTrigger(trigger Trigger) *DailyScheduleBuilder {
	hash := trigger.Hash()
	if _, ok := b.hashes[hash]; ok {
		b.errors = append(b.errors, fmt.Errorf("duplicate trigger: %v", trigger))
		return b
	}

	b.triggers = append(b.triggers, trigger)
	b.hashes[hash] = true

	return b
}

func (b *DailyScheduleBuilder) onSun(sunset bool, offset ...types.DurationString) *DailyScheduleBuilder {
	if len(offset) == 0 {
		b.errors = append(b.errors, fmt.Errorf("no offset provided for sun"))
		return b
	}

	offsetDuration, err := time.ParseDuration(string(offset[0]))
	if err != nil {
		b.errors = append(b.errors, err)
		return b
	}

	return b.tryAddTrigger(&SunTrigger{
		sunset: sunset,
		offset: &offsetDuration,
	})
}

// OnSunrise adds a trigger for sunrise with an optional offset.
// Only the first offset is considered.
// You can call this multiple times to add multiple triggers for sunrise with different offsets.
func (b *DailyScheduleBuilder) OnSunrise(offset ...types.DurationString) *DailyScheduleBuilder {
	return b.onSun(false, offset...)
}

// OnSunset adds a trigger for sunset with an optional offset.
// Only the first offset is considered.
func (b *DailyScheduleBuilder) OnSunset(offset ...types.DurationString) *DailyScheduleBuilder {
	return b.onSun(true, offset...)
}

// OnFixedTime adds a trigger for a fixed time each day.
// The time is in the local timezone.
// This will error if the integer values are not in the range 0-23 for the hour and 0-59 for the minute.
func (b *DailyScheduleBuilder) OnFixedTime(hour, minute int) *DailyScheduleBuilder {
	errored := false
	if hour < 0 || hour > 23 {
		b.errors = append(b.errors, fmt.Errorf("hour must be between 0 and 23"))
		errored = true
	}

	if minute < 0 || minute > 59 {
		b.errors = append(b.errors, fmt.Errorf("minute must be between 0 and 59"))
		errored = true
	}

	if errored {
		return b
	}

	return b.tryAddTrigger(&FixedTimeTrigger{
		Hour:   hour,
		Minute: minute,
	})
}

// Build returns a Trigger that will trigger at the configured times.
// It will return an error if any errors occurred during configuration.
func (b *DailyScheduleBuilder) Build() (Trigger, error) {
	// If there are no triggers, add an error.
	if len(b.triggers) == 0 {
		b.errors = append(b.errors, fmt.Errorf("no triggers provided"))
	}

	// If there are errors, return an error.
	if len(b.errors) > 0 {
		return nil, fmt.Errorf("errors occurred: %v", b.errors)
	}

	// If there is only one trigger, return it.
	if len(b.triggers) == 1 {
		return b.triggers[0], nil
	}

	// Otherwise, return a composite schedule that combines all the triggers.
	return &CompositeDailySchedule{triggers: b.triggers}, nil
}
