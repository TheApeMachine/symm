package replay

import (
	"runtime"
	"sync"

	"github.com/theapemachine/symm/market"
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

func precompileWorkerCount(workers int) int {
	if workers > 0 {
		return workers
	}

	return runtime.NumCPU()
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

	symbolIndices, globalIndices, categories, lastPrices := mergePrecompileChunks(chunks)

	if err := precompileSnapshotIndices(ticks, symbolIndices, globalIndices, workers); err != nil {
		return ReplayTape{}, err
	}

	categoryCount := len(categories)

	if categoryCount <= 0 {
		categoryCount = 1
	}

	return ReplayTape{
		Ticks:               ticks,
		LastPrices:          lastPrices,
		ReentryTickCooldown: deriveReentryTickCooldown(rowCount, categoryCount),
		MedianInterval:      medianMeasurementInterval(rows),
	}, nil
}

func precompileTapeSequential(rows []types.Measurement) (ReplayTape, error) {
	ticks := make([]PrecompiledTick, len(rows))
	lastPrices := make(map[string]float64)
	categories := make(map[types.CategoryType]struct{})
	symbolIndices := make(map[string][]int)
	globalIndices := make([]int, 0)

	for index, row := range rows {
		ticks[index] = PrecompiledTick{Row: row}

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

	if err := precompileSnapshotIndices(ticks, symbolIndices, globalIndices, 1); err != nil {
		return ReplayTape{}, err
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
	}, nil
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

	for index := startIndex; index < endIndex; index++ {
		row := rows[index]
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

func precompileSnapshotIndices(
	ticks []PrecompiledTick,
	symbolIndices map[string][]int,
	globalIndices []int,
	workers int,
) error {
	measurementBuffer, err := market.MeasurementBuffer()

	if err != nil {
		return err
	}

	workers = precompileWorkerCount(workers)
	rowCount := len(ticks)

	if rowCount < parallelPrecompileRowThreshold || workers <= 1 {
		for index := range ticks {
			if ticks[index].Row.Symbol == "" {
				continue
			}

			ticks[index].SnapshotIndices = mergeSnapshotIndices(
				ticks,
				symbolIndices,
				globalIndices,
				index,
				measurementBuffer,
			)
		}

		return nil
	}

	if workers > rowCount {
		workers = rowCount
	}

	var passTwo sync.WaitGroup
	chunkSize := (rowCount + workers - 1) / workers

	for workerIndex := range workers {
		startIndex := workerIndex * chunkSize
		endIndex := startIndex + chunkSize

		if startIndex >= rowCount {
			break
		}

		if endIndex > rowCount {
			endIndex = rowCount
		}

		passTwo.Add(1)

		go func(startIndex, endIndex int) {
			defer passTwo.Done()

			for index := startIndex; index < endIndex; index++ {
				if ticks[index].Row.Symbol == "" {
					continue
				}

				ticks[index].SnapshotIndices = mergeSnapshotIndices(
					ticks,
					symbolIndices,
					globalIndices,
					index,
					measurementBuffer,
				)
			}
		}(startIndex, endIndex)
	}

	passTwo.Wait()

	return nil
}
