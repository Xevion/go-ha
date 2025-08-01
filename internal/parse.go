package internal

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-module/carbon"
)

// Parses a HH:MM string.
func ParseTime(s string) carbon.Carbon {
	t, err := time.Parse("15:04", s)
	if err != nil {
		parsingErr := fmt.Errorf("failed to parse time string \"%s\"; format must be HH:MM.: %w", s, err)
		slog.Error(parsingErr.Error())
		panic(parsingErr)
	}
	return carbon.Now().SetTimeMilli(t.Hour(), t.Minute(), 0, 0)
}

func ParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		parsingErr := fmt.Errorf("couldn't parse string duration: \"%s\" see https://pkg.go.dev/time#ParseDuration for valid time units: %w", s, err)
		slog.Error(parsingErr.Error())
		panic(parsingErr)
	}
	return d
}
