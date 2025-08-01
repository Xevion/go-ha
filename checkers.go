package gomeassistant

import (
	"time"

	"github.com/Xevion/go-ha/internal"
	"github.com/Xevion/go-ha/types"
	"github.com/golang-module/carbon"
)

type ConditionCheck struct {
	fail bool
}

func CheckWithinTimeRange(startTime, endTime string) ConditionCheck {
	cc := ConditionCheck{fail: false}
	// if betweenStart and betweenEnd both set, first account for midnight
	// overlap, then check if between those times.
	if startTime != "" && endTime != "" {
		parsedStart := internal.ParseTime(startTime)
		parsedEnd := internal.ParseTime(endTime)

		// check for midnight overlap
		if parsedEnd.Lt(parsedStart) { // example turn on night lights when motion from 23:00 to 07:00
			if parsedEnd.IsPast() { // such as at 15:00, 22:00
				parsedEnd = parsedEnd.AddDay()
			} else {
				parsedStart = parsedStart.SubDay() // such as at 03:00, 05:00
			}
		}

		// skip callback if not inside the range
		if !carbon.Now().BetweenIncludedStart(parsedStart, parsedEnd) {
			cc.fail = true
		}

		// otherwise just check individual before/after
	} else if startTime != "" && internal.ParseTime(startTime).IsFuture() {
		cc.fail = true
	} else if endTime != "" && internal.ParseTime(endTime).IsPast() {
		cc.fail = true
	}
	return cc
}

func CheckStatesMatch(listenerState, s string) ConditionCheck {
	cc := ConditionCheck{fail: false}
	// check if fromState or toState are set and don't match
	if listenerState != "" && listenerState != s {
		cc.fail = true
	}
	return cc
}

func CheckThrottle(throttle time.Duration, lastRan carbon.Carbon) ConditionCheck {
	cc := ConditionCheck{fail: false}
	// check if Throttle is set and that duration hasn't passed since lastRan
	if throttle.Seconds() > 0 &&
		lastRan.DiffAbsInSeconds(carbon.Now()) < int64(throttle.Seconds()) {
		cc.fail = true
	}
	return cc
}

func CheckExceptionDates(eList []time.Time) ConditionCheck {
	cc := ConditionCheck{fail: false}
	for _, e := range eList {
		y1, m1, d1 := e.Date()
		y2, m2, d2 := time.Now().Date()
		if y1 == y2 && m1 == m2 && d1 == d2 {
			cc.fail = true
			break
		}
	}
	return cc
}

func CheckExceptionRanges(eList []types.TimeRange) ConditionCheck {
	cc := ConditionCheck{fail: false}
	now := time.Now()
	for _, eRange := range eList {
		if now.After(eRange.Start) && now.Before(eRange.End) {
			cc.fail = true
			break
		}
	}
	return cc
}

func CheckEnabledEntity(s State, infos []internal.EnabledDisabledInfo) ConditionCheck {
	cc := ConditionCheck{fail: false}
	if len(infos) == 0 {
		return cc
	}

	for _, edi := range infos {
		matches, err := s.Equals(edi.Entity, edi.State)

		if err != nil {
			if edi.RunOnError {
				// keep checking
				continue
			} else {
				// don't run this automation
				cc.fail = true
				break
			}
		}

		if !matches {
			cc.fail = true
			break
		}
	}
	return cc
}

func CheckDisabledEntity(s State, infos []internal.EnabledDisabledInfo) ConditionCheck {
	cc := ConditionCheck{fail: false}
	if len(infos) == 0 {
		return cc
	}

	for _, edi := range infos {
		matches, err := s.Equals(edi.Entity, edi.State)

		if err != nil {
			if edi.RunOnError {
				// keep checking
				continue
			} else {
				// don't run this automation
				cc.fail = true
				break
			}
		}

		if matches {
			cc.fail = true
			break
		}
	}

	return cc
}

func CheckAllowlistDates(eList []time.Time) ConditionCheck {
	if len(eList) == 0 {
		return ConditionCheck{fail: false}
	}

	cc := ConditionCheck{fail: true}
	for _, e := range eList {
		y1, m1, d1 := e.Date()
		y2, m2, d2 := time.Now().Date()
		if y1 == y2 && m1 == m2 && d1 == d2 {
			cc.fail = false
			break
		}
	}
	return cc
}

func CheckStartEndTime(s types.TimeString, isStart bool) ConditionCheck {
	cc := ConditionCheck{fail: false}
	// pass immediately if default
	if s == "00:00" {
		return cc
	}

	now := time.Now()
	parsedTime := internal.ParseTime(string(s)).Carbon2Time()
	if isStart {
		if parsedTime.After(now) {
			cc.fail = true
		}
	} else {
		if parsedTime.Before(now) {
			cc.fail = true
		}
	}
	return cc
}
