package market

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
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
) map[string]*datura.Artifact {
	origins := make(map[string]*datura.Artifact)

	if story == nil || story.symbols == nil || symbol == "" {
		return origins
	}

	value, ok := story.symbols.Load(symbol)
	if !ok {
		return origins
	}

	ring, ok := value.(*structure.ListRing[*datura.Artifact])
	if !ok || ring == nil {
		return origins
	}

	ring.Do(func(measurement *datura.Artifact) {
		if !activeMeasurement(measurement, now, maxAge) {
			return
		}

		origin := datura.Peek[string](measurement, "origin")
		current := origins[origin]
		if current == nil || measurement.Timestamp() >= current.Timestamp() {
			origins[origin] = measurement
		}
	})

	return origins
}

func activeMeasurement(
	measurement *datura.Artifact,
	now time.Time,
	maxAge time.Duration,
) bool {
	if measurement == nil {
		return false
	}

	if datura.Peek[string](measurement, "origin") == "" {
		return false
	}

	if datura.Peek[float64](measurement, "output", "confidence") <= 0 {
		return false
	}

	if maxAge <= 0 {
		return true
	}

	timestamp := time.Unix(0, measurement.Timestamp())
	return !timestamp.IsZero() && !timestamp.After(now) && now.Sub(timestamp) <= maxAge
}
