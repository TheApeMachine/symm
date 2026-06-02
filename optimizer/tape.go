package optimizer

import (
	"time"

	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
StoryRingCapacity matches market.Story's measurement ring buffer. Each replay
decision sees at most this many recent measurements — the live search space.
*/
const StoryRingCapacity = market.StoryRingCapacity

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
	Ticks               []PrecompiledTick
	LastPrices          map[string]float64
	ReentryTickCooldown int
	MedianInterval      time.Duration
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
	categories := make(map[perspectives.CategoryType]struct{})

	for index, row := range rows {
		ring = market.AppendRingMeasurement(ring, row)

		if row.Category != perspectives.CategoryTypeNone {
			categories[row.Category] = struct{}{}
		}

		if row.Symbol == "" {
			continue
		}

		if row.Last > 0 {
			lastPrices[row.Symbol] = row.Last
		}

		ticks[index] = PrecompiledTick{
			Row:       row,
			Snapshots: market.RingSnapshot(ring, row.Symbol),
		}
	}

	categoryCount := len(categories)

	if categoryCount <= 0 {
		categoryCount = 1
	}

	return ReplayTape{
		Ticks:               ticks,
		LastPrices:          lastPrices,
		ReentryTickCooldown: deriveReentryTickCooldown(len(rows), categoryCount),
		MedianInterval:      medianMeasurementInterval(rows),
	}
}
