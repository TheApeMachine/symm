package replay

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
PrecompiledTick holds one replay row and the tick indices that form its decision
snapshot window. Indices are resolved once during precompilation so simulation does
not repeat binary searches and merge passes on every candidate replay.
*/
type PrecompiledTick struct {
	Row             types.Measurement
	SnapshotIndices []int
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
AppendSnapshot appends the live-story ring snapshot for tickIndex into
destination: the last story.measurements.buffer measurements OF THE TICK'S OWN
SYMBOL, chronologically. This mirrors market/story.rememberMeasurement exactly —
a per-symbol ring of that symbol's readings (symbol-less rows never enter a live
window). The previous scheme windowed by GLOBAL tick distance, which on a
multi-symbol tape gave each symbol ~buffer/symbolCount rows of context while live
saw up to the full buffer — regimes and lookbacks diverged between the clocks.
*/
func (tape ReplayTape) AppendSnapshot(
	tickIndex int,
	destination []types.Measurement,
) []types.Measurement {
	if tickIndex < 0 || tickIndex >= len(tape.Ticks) {
		return destination[:0]
	}

	indices := tape.Ticks[tickIndex].SnapshotIndices
	destination = destination[:0]

	for _, index := range indices {
		destination = append(destination, tape.Ticks[index].Row)
	}

	return destination
}

// indicesInWindow returns the sub-slice of indices whose values fall in
// [startIndex, endIndex]. indices must be sorted in ascending order (required
// by sort.SearchInts and sort.Search); behavior is undefined if unsorted.
func indicesInWindow(indices []int, startIndex, endIndex int) []int {
	if len(indices) == 0 {
		return nil
	}

	start := sort.SearchInts(indices, startIndex)
	end := sort.Search(len(indices), func(index int) bool {
		return indices[index] > endIndex
	})

	return indices[start:end]
}

func mergeSnapshotIndices(
	ticks []PrecompiledTick,
	symbolIndices map[string][]int,
	globalIndices []int,
	tickIndex int,
	measurementBuffer int,
) []int {
	_ = globalIndices // symbol-less rows never enter a live story window

	symbol := ticks[tickIndex].Row.Symbol

	if symbol == "" {
		return nil
	}

	indices := symbolIndices[symbol]

	// Window by OCCURRENCE COUNT of this symbol, not by global tick distance:
	// the live ring holds the symbol's last measurementBuffer readings however
	// long ago they happened.
	end := sort.Search(len(indices), func(index int) bool {
		return indices[index] > tickIndex
	})

	start := end - measurementBuffer

	if start < 0 {
		start = 0
	}

	if start >= end {
		return nil
	}

	merged := make([]int, end-start)
	copy(merged, indices[start:end])

	return merged
}

/*
PrecompileTape builds compact replay state matching market.Story decision context.
*/
func PrecompileTape(rows []types.Measurement) (ReplayTape, error) {
	return PrecompileTapeWorkers(rows, 0)
}
