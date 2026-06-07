package replay

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
PrecompiledTick holds one replay row plus its ordinal position within its
symbol's occurrence list. The decision snapshot window is DERIVED from that one
integer at read time — materializing up to story-ring-size indices per tick (the
previous scheme) cost ~8KB × tick on busy symbols and OOM-killed precompilation
on an hour of capture.
*/
type PrecompiledTick struct {
	Row           types.Measurement
	SymbolOrdinal int // position of this tick within symbolIndices[Row.Symbol]
	Regime        types.Regime // ClassifyRegime at precompile; identical on every candidate replay
}

/*
ReplayTape is market state compiled once and shared across candidate scoring.
*/
type ReplayTape struct {
	Ticks               []PrecompiledTick
	SymbolIndices       map[string][]int // per symbol: tick indices in chronological order
	LastPrices          map[string]float64
	ReentryTickCooldown int
	MedianInterval      time.Duration
	measurementBuffer   int
}

func (tape ReplayTape) Len() int {
	return len(tape.Ticks)
}

/*
AppendSnapshot appends the live-story ring snapshot for tickIndex into
destination: the last story.measurements.buffer measurements OF THE TICK'S OWN
SYMBOL, chronologically — derived from the tick's ordinal in its symbol's
occurrence list, allocating nothing. This mirrors market/story.rememberMeasurement
exactly (symbol-less rows never enter a live window).
*/
func (tape ReplayTape) AppendSnapshot(
	tickIndex int,
	destination []types.Measurement,
) []types.Measurement {
	destination = destination[:0]

	if tickIndex < 0 || tickIndex >= len(tape.Ticks) {
		return destination
	}

	symbol := tape.Ticks[tickIndex].Row.Symbol

	if symbol == "" {
		return destination
	}

	indices := tape.SymbolIndices[symbol]
	end := tape.Ticks[tickIndex].SymbolOrdinal + 1

	if end <= 0 || end > len(indices) {
		return destination
	}

	start := end - tape.measurementBuffer

	// Non-positive measurementBuffer yields an empty window.
	if tape.measurementBuffer <= 0 {
		start = end
	}

	if start < 0 {
		start = 0
	}

	for _, index := range indices[start:end] {
		destination = append(destination, tape.Ticks[index].Row)
	}

	return destination
}

/*
PrecompileTape builds compact replay state matching market.Story decision context.
*/
func PrecompileTape(rows []types.Measurement) (ReplayTape, error) {
	return PrecompileTapeWorkers(rows, 0)
}
