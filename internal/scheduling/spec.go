package scheduling

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// Spec describes a trigger that cannot be built until the observer location is
// known. Schedules are declared as package level values before an App exists,
// so sun triggers have nowhere to read coordinates from until registration.
type Spec interface {
	Resolve(location Location) (Trigger, error)
	Hash() uint64
	String() string
}

// sunLabel renders a sun event and its offset, e.g. "sunset" or "sunrise-30m".
func sunLabel(sunset bool, offset *time.Duration) string {
	name := "sunrise"
	if sunset {
		name = "sunset"
	}

	if offset == nil || *offset == 0 {
		return name
	}
	if *offset > 0 {
		return name + "+" + offset.String()
	}
	// Duration.String already carries the sign for negative offsets.
	return name + offset.String()
}

type fixedTimeSpec struct {
	hour   int
	minute int
}

func (s fixedTimeSpec) Resolve(Location) (Trigger, error) {
	return &FixedTimeTrigger{Hour: s.hour, Minute: s.minute}, nil
}

func (s fixedTimeSpec) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "fixed:%d:%d", s.hour, s.minute)
	return h.Sum64()
}

func (s fixedTimeSpec) String() string {
	return fmt.Sprintf("%02d:%02d", s.hour, s.minute)
}

type sunSpec struct {
	sunset bool
	offset *time.Duration
}

func (s sunSpec) Resolve(location Location) (Trigger, error) {
	return &SunTrigger{
		latitude:  location.Latitude,
		longitude: location.Longitude,
		sunset:    s.sunset,
		offset:    s.offset,
	}, nil
}

// Hash deliberately ignores location. Two sunset triggers with the same offset
// describe the same intent wherever they end up resolving.
func (s sunSpec) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "sun:%t", s.sunset)
	if s.offset != nil {
		fmt.Fprintf(h, ":%d", s.offset.Nanoseconds())
	}
	return h.Sum64()
}

func (s sunSpec) String() string {
	return sunLabel(s.sunset, s.offset)
}

type cronSpec struct {
	expression string
}

func (s cronSpec) Resolve(Location) (Trigger, error) {
	return NewCronTrigger(s.expression)
}

func (s cronSpec) Hash() uint64 {
	h := fnv.New64()
	fmt.Fprintf(h, "cron:%s", s.expression)
	return h.Sum64()
}

func (s cronSpec) String() string {
	return "cron(" + s.expression + ")"
}

type compositeSpec struct {
	specs []Spec
}

func (s compositeSpec) Resolve(location Location) (Trigger, error) {
	triggers := make([]Trigger, 0, len(s.specs))
	for _, spec := range s.specs {
		trigger, err := spec.Resolve(location)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, trigger)
	}
	return &CompositeDailySchedule{triggers: triggers}, nil
}

func (s compositeSpec) Hash() uint64 {
	h := fnv.New64()
	for _, spec := range s.specs {
		fmt.Fprintf(h, "%d", spec.Hash())
	}
	return h.Sum64()
}

func (s compositeSpec) String() string {
	parts := make([]string, 0, len(s.specs))
	for _, spec := range s.specs {
		parts = append(parts, spec.String())
	}
	return strings.Join(parts, ", ")
}
