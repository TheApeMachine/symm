package trader

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives"
)

const minMeasurementTTL = 250 * time.Millisecond

/*
timedMeasurement is the trader-local freshness wrapper around one signal's latest
verdict for a symbol. The trader stores only the newest category per source: a CVD
reading that moved from aggressive_drive to stochastic_balance replaces the old
category instead of leaving both in the active thesis set.
*/
type timedMeasurement struct {
	Measurement perspectives.Measurement
	At          time.Time
	TTL         time.Duration
}

/*
newTimedMeasurement stamps a measurement and derives its freshness from the source's
own observed cadence. Once the source has emitted twice for the same symbol, the
previous inter-arrival time becomes the live freshness window; until then the reading
is kept because there is no cadence to estimate yet.
*/
func newTimedMeasurement(
	measurement perspectives.Measurement,
	previous timedMeasurement,
) timedMeasurement {
	now := time.Now()
	ttl := previous.TTL

	if !previous.At.IsZero() {
		interval := now.Sub(previous.At)

		if interval > 0 {
			ttl = interval + interval
		}

		if ttl < minMeasurementTTL {
			ttl = minMeasurementTTL
		}
	}

	return timedMeasurement{Measurement: measurement, At: now, TTL: ttl}
}

/*
copyReadingSet clones one symbol's source map so cross-section workers can read
without racing record() updates on the live desk map.
*/
func copyReadingSet(set map[perspectives.SourceType]timedMeasurement) map[perspectives.SourceType]timedMeasurement {
	if len(set) == 0 {
		return nil
	}

	copySet := make(map[perspectives.SourceType]timedMeasurement, len(set))

	for source, slot := range set {
		copySet[source] = slot
	}

	return copySet
}

/*
snapshotTimedMeasurements returns the non-stale latest verdicts for a symbol.
*/
func snapshotTimedMeasurements(
	set map[perspectives.SourceType]timedMeasurement,
	now time.Time,
) []perspectives.Measurement {
	measurements := make([]perspectives.Measurement, 0, len(set))

	for _, slot := range set {
		if slot.Stale(now) {
			continue
		}

		measurements = append(measurements, slot.Measurement)
	}

	return measurements
}

/*
Stale reports whether a slot has missed the cadence-derived freshness window.
*/
func (slot timedMeasurement) Stale(now time.Time) bool {
	if slot.TTL <= 0 || slot.At.IsZero() {
		return false
	}

	return now.Sub(slot.At) > slot.TTL
}
