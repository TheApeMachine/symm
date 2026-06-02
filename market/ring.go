package market

import "github.com/theapemachine/symm/market/perspectives"

/*
StoryRingCapacity matches the live story measurement window. Each walk sees at
most this many recent measurements — the same replay search space as the optimizer.
*/
const StoryRingCapacity = 128

/*
AppendRingMeasurement appends one row and trims to StoryRingCapacity.
*/
func AppendRingMeasurement(
	ring []perspectives.Measurement, row perspectives.Measurement,
) []perspectives.Measurement {
	ring = append(ring, row)

	if len(ring) <= StoryRingCapacity {
		return ring
	}

	return ring[len(ring)-StoryRingCapacity:]
}

/*
RingSnapshot returns measurements for symbol plus global rows from the ring window.
*/
func RingSnapshot(
	ring []perspectives.Measurement, symbol string,
) []perspectives.Measurement {
	snapshots := make([]perspectives.Measurement, 0, len(ring))

	for _, measurement := range ring {
		if measurement.Symbol != "" && measurement.Symbol != symbol {
			continue
		}

		snapshots = append(snapshots, measurement)
	}

	return snapshots
}
