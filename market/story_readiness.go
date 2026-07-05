package market

import (
	"time"

	"github.com/theapemachine/symm/logic"
)

/*
ActiveOrigins returns the latest fresh, confident measurement per origin for a
symbol. Trader readiness uses this view so stale or standby measurements cannot
price an entry.
*/
func (story *Story) ActiveOrigins(
	symbol string,
	now time.Time,
	maxAge time.Duration,
) map[string]*logic.Measurement {
	origins := make(map[string]*logic.Measurement)

	if story == nil || story.symbols == nil || symbol == "" {
		return origins
	}

	value, ok := story.symbols.Load(symbol)
	if !ok {
		return origins
	}

	scope, ok := value.(*storySymbol)
	if !ok || scope == nil {
		return origins
	}

	for _, measurement := range scope.measurements {
		if !activeMeasurement(measurement, now, maxAge) {
			continue
		}

		current := origins[string(measurement.Source)]
		if current == nil || measurement.At.After(current.At) || measurement.At.Equal(current.At) {
			origins[string(measurement.Source)] = measurement
		}
	}

	return origins
}

func activeMeasurement(
	measurement *logic.Measurement,
	now time.Time,
	maxAge time.Duration,
) bool {
	if measurement == nil || measurement.Source == logic.SourceNone {
		return false
	}

	if measurement.Confidence <= 0 {
		return false
	}

	switch measurement.Status {
	case "standby", "ambiguous", "calibrating":
		return false
	}

	if maxAge <= 0 {
		return true
	}

	timestamp := measurement.At
	return !timestamp.IsZero() && !timestamp.After(now) && now.Sub(timestamp) <= maxAge
}
