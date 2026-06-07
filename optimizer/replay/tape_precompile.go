package replay

import (
	"runtime"
	"sync"

	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// parallelPrecompileRowThreshold is the minimum tape length before precompilation
// uses multiple workers. Smaller tapes stay sequential to avoid overhead.
const parallelPrecompileRowThreshold = 8192

type precompileChunk struct {
	symbolIndices map[string][]int
	globalIndices []int
	categories    map[types.CategoryType]struct{}
	lastPrices    map[string]float64
}

// sharedBooks aliases a row's book slices to the symbol's previous tick when
// byte-identical. Half of every capture is prediction rows carrying exact
// copies of their parent row's book; sharing halves resident book memory.
type sharedBooks struct {
	bids []types.BookLevel
	asks []types.BookLevel
}

func shareRowBooks(row *types.Measurement, previous map[string]sharedBooks) {
	if row.Symbol == "" {
		return
	}

	prior, ok := previous[row.Symbol]

	if ok {
		if bookLevelsEqual(row.BookBids, prior.bids) {
			row.BookBids = prior.bids
		}

		if bookLevelsEqual(row.BookAsks, prior.asks) {
			row.BookAsks = prior.asks
		}
	}

	previous[row.Symbol] = sharedBooks{bids: row.BookBids, asks: row.BookAsks}
}

func bookLevelsEqual(left, right []types.BookLevel) bool {
	if len(left) != len(right) || len(left) == 0 {
		return false
	}

	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}

func precompileWorkerCount(workers int) int {
	if workers > 0 {
		return workers
	}

	return runtime.NumCPU()
}

/*
backfillSymbolQuotes gives every row its symbol's last-known book and top-of-book,
aliased (never copied). This is the replay twin of the live quote cache: the desk
quotes an order from the cache's CURRENT book, not from whatever book happened to
ride on the firing measurement — so rows recorded without books (prediction rows,
cache-gap rows) must still present the book a live desk would have used, or
replay refuses entries live would take. Aliasing keeps resident memory at one
book per change instead of one per row.
*/
func backfillSymbolQuotes(rows []types.Measurement) {
	lastBooks := make(map[string]sharedBooks)
	type topOfBook struct{ bid, ask float64 }
	lastTops := make(map[string]topOfBook)

	for index := range rows {
		symbol := rows[index].Symbol

		if symbol == "" {
			continue
		}

		if rows[index].HasBookDepth() {
			lastBooks[symbol] = sharedBooks{bids: rows[index].BookBids, asks: rows[index].BookAsks}
		} else if prior, ok := lastBooks[symbol]; ok {
			rows[index].BookBids = prior.bids
			rows[index].BookAsks = prior.asks
		}

		if rows[index].Bid > 0 || rows[index].Ask > 0 {
			lastTops[symbol] = topOfBook{bid: rows[index].Bid, ask: rows[index].Ask}
		} else if prior, ok := lastTops[symbol]; ok {
			rows[index].Bid = prior.bid
			rows[index].Ask = prior.ask
		}
	}
}

/*
PrecompileTapeWorkers builds replay state with parallel index and snapshot passes.
Pass one to force a fully sequential build.
*/
func PrecompileTapeWorkers(rows []types.Measurement, workers int) (ReplayTape, error) {
	rowCount := len(rows)

	if rowCount == 0 {
		return ReplayTape{}, nil
	}

	backfillSymbolQuotes(rows)

	workers = precompileWorkerCount(workers)

	if rowCount < parallelPrecompileRowThreshold || workers <= 1 {
		return precompileTapeSequential(rows)
	}

	if workers > rowCount {
		workers = rowCount
	}

	ticks := make([]PrecompiledTick, rowCount)
	chunks := make([]precompileChunk, workers)
	chunkSize := (rowCount + workers - 1) / workers

	var passOne sync.WaitGroup

	for workerIndex := range workers {
		startIndex := workerIndex * chunkSize
		endIndex := startIndex + chunkSize

		if startIndex >= rowCount {
			break
		}

		if endIndex > rowCount {
			endIndex = rowCount
		}

		passOne.Add(1)

		go func(workerIndex, startIndex, endIndex int) {
			defer passOne.Done()

			chunks[workerIndex] = buildPrecompileChunk(rows, ticks, startIndex, endIndex)
		}(workerIndex, startIndex, endIndex)
	}

	passOne.Wait()

	symbolIndices, _, categories, lastPrices := mergePrecompileChunks(chunks)
	buffer, err := assignSymbolOrdinals(ticks, symbolIndices)

	if err != nil {
		return ReplayTape{}, err
	}

	categoryCount := len(categories)

	if categoryCount <= 0 {
		categoryCount = 1
	}

	tape := ReplayTape{
		Ticks:               ticks,
		SymbolIndices:       symbolIndices,
		LastPrices:          lastPrices,
		ReentryTickCooldown: deriveReentryTickCooldown(rowCount, categoryCount),
		MedianInterval:      medianMeasurementInterval(rows),
		measurementBuffer:   buffer,
	}
	stampPrecompiledRegimes(&tape)

	return tape, nil
}

func precompileTapeSequential(rows []types.Measurement) (ReplayTape, error) {
	ticks := make([]PrecompiledTick, len(rows))
	lastPrices := make(map[string]float64)
	categories := make(map[types.CategoryType]struct{})
	symbolIndices := make(map[string][]int)
	globalIndices := make([]int, 0)

	previousBooks := make(map[string]sharedBooks)

	for index, row := range rows {
		quoted := QuotedMeasurement(row)
		shareRowBooks(&quoted, previousBooks)
		ticks[index] = PrecompiledTick{Row: quoted}

		if row.Category != types.CategoryTypeNone {
			categories[row.Category] = struct{}{}
		}

		if row.Symbol == "" {
			globalIndices = append(globalIndices, index)

			continue
		}

		if row.Last > 0 {
			lastPrices[row.Symbol] = row.Last
		}

		symbolIndices[row.Symbol] = append(symbolIndices[row.Symbol], index)
	}

	_ = globalIndices // symbol-less rows never enter a live story window

	buffer, err := assignSymbolOrdinals(ticks, symbolIndices)

	if err != nil {
		return ReplayTape{}, err
	}

	categoryCount := len(categories)

	if categoryCount <= 0 {
		categoryCount = 1
	}

	tape := ReplayTape{
		Ticks:               ticks,
		SymbolIndices:       symbolIndices,
		LastPrices:          lastPrices,
		ReentryTickCooldown: deriveReentryTickCooldown(len(rows), categoryCount),
		MedianInterval:      medianMeasurementInterval(rows),
		measurementBuffer:   buffer,
	}
	stampPrecompiledRegimes(&tape)

	return tape, nil
}

func buildPrecompileChunk(
	rows []types.Measurement,
	ticks []PrecompiledTick,
	startIndex, endIndex int,
) precompileChunk {
	chunk := precompileChunk{
		symbolIndices: make(map[string][]int),
		globalIndices: make([]int, 0, (endIndex-startIndex)/8),
		categories:    make(map[types.CategoryType]struct{}),
		lastPrices:    make(map[string]float64),
	}

	previousBooks := make(map[string]sharedBooks)

	for index := startIndex; index < endIndex; index++ {
		row := QuotedMeasurement(rows[index])
		shareRowBooks(&row, previousBooks)
		ticks[index] = PrecompiledTick{Row: row}

		if row.Category != types.CategoryTypeNone {
			chunk.categories[row.Category] = struct{}{}
		}

		if row.Symbol == "" {
			chunk.globalIndices = append(chunk.globalIndices, index)

			continue
		}

		if row.Last > 0 {
			chunk.lastPrices[row.Symbol] = row.Last
		}

		chunk.symbolIndices[row.Symbol] = append(chunk.symbolIndices[row.Symbol], index)
	}

	return chunk
}

func mergePrecompileChunks(chunks []precompileChunk) (
	map[string][]int,
	[]int,
	map[types.CategoryType]struct{},
	map[string]float64,
) {
	symbolIndices := make(map[string][]int)
	globalIndices := make([]int, 0)
	categories := make(map[types.CategoryType]struct{})
	lastPrices := make(map[string]float64)

	for _, chunk := range chunks {
		if chunk.symbolIndices == nil {
			continue
		}

		for category := range chunk.categories {
			categories[category] = struct{}{}
		}

		for symbol, price := range chunk.lastPrices {
			lastPrices[symbol] = price
		}

		// Each chunk.globalIndices is sorted ascending and chunks are merged in
		// contiguous row order, so appending preserves chronological order in globalIndices.
		globalIndices = append(globalIndices, chunk.globalIndices...)

		for symbol, indices := range chunk.symbolIndices {
			symbolIndices[symbol] = append(symbolIndices[symbol], indices...)
		}
	}

	return symbolIndices, globalIndices, categories, lastPrices
}

/*
assignSymbolOrdinals stamps each tick with its position in its symbol's
occurrence list and returns the configured story ring size. The decision window
is derived from the ordinal at read time — nothing per-tick is materialized.
*/
func assignSymbolOrdinals(
	ticks []PrecompiledTick,
	symbolIndices map[string][]int,
) (int, error) {
	buffer, err := market.MeasurementBuffer()

	if err != nil {
		return 0, err
	}

	for _, indices := range symbolIndices {
		for ordinal, tickIndex := range indices {
			ticks[tickIndex].SymbolOrdinal = ordinal
		}
	}

	return buffer, nil
}

/*
stampPrecompiledRegimes stores ClassifyRegime once per tick so candidate scoring
does not re-derive the same price-action state on every forest replay.
*/
func stampPrecompiledRegimes(tape *ReplayTape) {
	scratch := make([]types.Measurement, 0, tape.measurementBuffer)

	for tickIndex := range tape.Ticks {
		tick := &tape.Ticks[tickIndex]

		if tick.Row.Symbol == "" {
			tick.Regime = types.RegimeNone

			continue
		}

		scratch = tape.AppendSnapshot(tickIndex, scratch)
		tick.Regime = perspectives.ClassifyRegime(scratch).Regime
	}
}
