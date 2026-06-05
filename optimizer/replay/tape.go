package replay

import (
	"sort"
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
PrecompiledTick holds one replay row and the tick indices that form its decision
snapshot window. Indices are resolved once during precompilation so simulation does
not repeat binary searches and merge passes on every candidate replay.
*/
type PrecompiledTick struct {
	Row             perspectives.Measurement
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
AppendSnapshot appends the exact live-story ring snapshot for tickIndex into
destination. It returns chronological rows from the last StoryRingCapacity ticks
whose symbol is either global or the tick symbol.
*/
func (tape ReplayTape) AppendSnapshot(
	tickIndex int,
	destination []perspectives.Measurement,
) []perspectives.Measurement {
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
) []int {
	symbol := ticks[tickIndex].Row.Symbol

	if symbol == "" {
		return nil
	}

	startIndex := tickIndex - StoryRingCapacity + 1

	if startIndex < 0 {
		startIndex = 0
	}

	symbolWindow := indicesInWindow(symbolIndices[symbol], startIndex, tickIndex)
	globalWindow := indicesInWindow(globalIndices, startIndex, tickIndex)
	merged := make([]int, 0, len(symbolWindow)+len(globalWindow))

	symbolCursor := 0
	globalCursor := 0

	for symbolCursor < len(symbolWindow) || globalCursor < len(globalWindow) {
		if globalCursor >= len(globalWindow) {
			merged = append(merged, symbolWindow[symbolCursor])
			symbolCursor++

			continue
		}

		if symbolCursor >= len(symbolWindow) {
			merged = append(merged, globalWindow[globalCursor])
			globalCursor++

			continue
		}

		if globalWindow[globalCursor] < symbolWindow[symbolCursor] {
			merged = append(merged, globalWindow[globalCursor])
			globalCursor++

			continue
		}

		merged = append(merged, symbolWindow[symbolCursor])
		symbolCursor++
	}

	return merged
}

/*
PrecompileTape builds compact replay state matching market.Story decision context.
*/
func PrecompileTape(rows []perspectives.Measurement) ReplayTape {
	return PrecompileTapeWorkers(rows, 0)
}
