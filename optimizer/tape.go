package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
StoryRingCapacity matches market.Story's measurement ring buffer. Each replay
decision sees at most this many recent measurements — the live search space.
*/
const StoryRingCapacity = 128

/*
PrecompiledTick holds one replay row and the ring-window snapshot at that moment.
*/
type PrecompiledTick struct {
	Row       perspectives.Measurement
	Snapshots []perspectives.Measurement
}

/*
ReplayTape is market state compiled once and shared across candidate scoring.
*/
type ReplayTape struct {
	Ticks      []PrecompiledTick
	LastPrices map[string]float64
}

func (tape ReplayTape) Len() int {
	return len(tape.Ticks)
}

/*
PrecompileTape builds per-tick ring snapshots matching market.Story decision context.
*/
func PrecompileTape(rows []perspectives.Measurement) ReplayTape {
	ring := make([]perspectives.Measurement, 0, StoryRingCapacity)
	ticks := make([]PrecompiledTick, len(rows))
	lastPrices := make(map[string]float64)

	for index, row := range rows {
		ring = appendRingMeasurement(ring, row)

		if row.Symbol == "" {
			continue
		}

		if row.Last > 0 {
			lastPrices[row.Symbol] = row.Last
		}

		ticks[index] = PrecompiledTick{
			Row:       row,
			Snapshots: ringSnapshot(ring, row.Symbol),
		}
	}

	return ReplayTape{
		Ticks:      ticks,
		LastPrices: lastPrices,
	}
}

func appendRingMeasurement(
	ring []perspectives.Measurement, row perspectives.Measurement,
) []perspectives.Measurement {
	ring = append(ring, row)

	if len(ring) <= StoryRingCapacity {
		return ring
	}

	return ring[len(ring)-StoryRingCapacity:]
}

func ringSnapshot(
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
