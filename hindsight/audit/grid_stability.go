package audit

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

/*
AnalyzeGridStability develops independent grids from disjoint chronological
market segments using the production ChannelsFrom -> Stream.Deform path.

It reports the largest half-vs-half comparison plus repeated comparisons using
quarter-length windows. Every observed ARI is accompanied by a random
co-membership baseline that preserves the compared partition's region-size
multiset. No ARI value is converted into a health threshold.
*/
func AnalyzeGridStability(
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	healthyCells []MetricStat,
	permutations int,
) Stage3GridStability {
	_ = healthyCells // Population description belongs to Stage 1; production replay uses arrivals as observed.

	totalTicks := len(ticks)
	if totalTicks < 40 {
		return Stage3GridStability{
			SummaryText: "Insufficient ticks (need at least 40) for chronological grid stability.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}
	if permutations <= 0 {
		permutations = 50
	}

	half := totalTicks / 2
	primary := compareGridWindows(
		ticks[:half],
		ticks[half:],
		tickMeasurements,
		permutations,
		1791,
	)
	if primary.sharedCells < 2 {
		return Stage3GridStability{
			SummaryText: "Disjoint half-period grids did not share enough cells for agreement measurement.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	curve := make([]GridStabilityObservation, 0, 3)

	// Two independent quarter-vs-quarter comparisons when the sample supports them.
	quarter := totalTicks / 4
	if quarter >= 20 {
		for pair := 0; pair < 2; pair++ {
			start := pair * quarter * 2
			mid := start + quarter
			end := min(mid+quarter, totalTicks)
			if end-mid < 20 {
				continue
			}
			comparison := compareGridWindows(
				ticks[start:mid],
				ticks[mid:end],
				tickMeasurements,
				permutations,
				int64(1791+pair+1),
			)
			if comparison.sharedCells < 2 {
				continue
			}
			curve = append(curve, GridStabilityObservation{
				WindowTicks:  min(mid-start, end-mid),
				PairIndex:    pair,
				SharedCells:  comparison.sharedCells,
				Overlap:      comparison.overlap,
				AdjustedRand: comparison.ari,
				NullMeanARI:  comparison.nullMean,
			})
		}
	}

	curve = append(curve, GridStabilityObservation{
		WindowTicks:  min(half, totalTicks-half),
		PairIndex:    0,
		SharedCells:  primary.sharedCells,
		Overlap:      primary.overlap,
		AdjustedRand: primary.ari,
		NullMeanARI:  primary.nullMean,
	})

	summary := fmt.Sprintf(
		"Grid reproducibility measured across %d disjoint comparisons. "+
			"Largest half-vs-half: %d/%d cells, shared=%d (%.1f%%), ARI=%.3f, randomized mean ARI=%.3f.",
		len(curve),
		primary.gridA.CellCount, primary.gridB.CellCount,
		primary.sharedCells, primary.overlap*100,
		primary.ari, primary.nullMean,
	)

	return Stage3GridStability{
		GridA:                primary.gridA,
		GridB:                primary.gridB,
		SharedUniverse:       primary.sharedCells,
		OverlapFraction:      primary.overlap,
		RandIndex:            primary.rand,
		AdjustedRandIdx:      primary.ari,
		NullAdjustedRandMean: primary.nullMean,
		StabilityCurve:       curve,
		SummaryText:          summary,
		Status:               "MEASURED",
		Passed:               true,
	}
}

type gridComparison struct {
	gridA       GridPartitionStat
	gridB       GridPartitionStat
	sharedCells int
	overlap     float64
	rand        float64
	ari         float64
	nullMean    float64
}

func compareGridWindows(
	ticksA []int64,
	ticksB []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	seed int64,
) gridComparison {
	gridA := buildGrid(ticksA, tickMeasurements)
	gridB := buildGrid(ticksB, tickMeasurements)
	statA := evaluateGridPartition("Period A", gridA)
	statB := evaluateGridPartition("Period B", gridB)

	partitionsA := extractPartitions(gridA)
	partitionsB := extractPartitions(gridB)
	sharedKeys := make([]string, 0)
	for key := range partitionsA {
		if _, ok := partitionsB[key]; ok {
			sharedKeys = append(sharedKeys, key)
		}
	}
	sort.Strings(sharedKeys)

	maxUniverse := max(len(partitionsA), len(partitionsB))
	overlap := 0.0
	if maxUniverse > 0 {
		overlap = float64(len(sharedKeys)) / float64(maxUniverse)
	}

	rawRand, ari := computeRandIndices(partitionsA, partitionsB, sharedKeys)
	nullMean := randomizedPartitionARIMean(
		partitionsA, partitionsB, sharedKeys, permutations, seed,
	)

	return gridComparison{
		gridA: statA, gridB: statB,
		sharedCells: len(sharedKeys),
		overlap: overlap,
		rand: rawRand,
		ari: ari,
		nullMean: nullMean,
	}
}

func buildGrid(
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) *store.Grid {
	stream := store.NewStream()
	grid := store.NewGrid()
	feedGridFaithful(grid, stream, ticks, tickMeasurements)
	grid.Partition()
	return grid
}

func randomizedPartitionARIMean(
	partA map[string]uint8,
	partB map[string]uint8,
	sharedKeys []string,
	permutations int,
	seed int64,
) float64 {
	if len(sharedKeys) < 2 || permutations <= 0 {
		return 0
	}

	labels := make([]uint8, len(sharedKeys))
	for index, key := range sharedKeys {
		labels[index] = partB[key]
	}

	rng := rand.New(rand.NewSource(seed))
	total := 0.0
	for iteration := 0; iteration < permutations; iteration++ {
		shuffled := append([]uint8(nil), labels...)
		rng.Shuffle(len(shuffled), func(first, second int) {
			shuffled[first], shuffled[second] = shuffled[second], shuffled[first]
		})

		randomB := make(map[string]uint8, len(sharedKeys))
		for index, key := range sharedKeys {
			randomB[key] = shuffled[index]
		}

		_, ari := computeRandIndices(partA, randomB, sharedKeys)
		total += ari
	}

	return total / float64(permutations)
}

func feedGridFaithful(
	grid *store.Grid,
	stream *store.Stream,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) {
	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		observed := strategy.ChannelsFrom(measGroup...)
		deformations := stream.Deform(observed.Raw)
		if len(deformations) > 0 {
			grid.Update(tick, deformations)
		}
	}
}

func evaluateGridPartition(name string, grid *store.Grid) GridPartitionStat {
	cells := grid.CellsSnapshot()
	cellCount := len(cells)
	if cellCount == 0 {
		return GridPartitionStat{
			PeriodName:   name,
			IsDegenerate: true,
		}
	}

	regionSizes := make(map[string]int)
	for _, cell := range cells {
		if cell != nil {
			regionSizes[fmt.Sprintf("R%d", cell.Region)]++
		}
	}

	maxShare := 0.0
	for _, size := range regionSizes {
		share := float64(size) / float64(cellCount)
		if share > maxShare {
			maxShare = share
		}
	}

	return GridPartitionStat{
		PeriodName:     name,
		CellCount:      cellCount,
		RegionCount:    len(regionSizes),
		RegionSizes:    regionSizes,
		MaxRegionShare: maxShare,
		IsDegenerate:   len(regionSizes) <= 1,
	}
}

func extractPartitions(grid *store.Grid) map[string]uint8 {
	result := make(map[string]uint8)
	for _, cell := range grid.CellsSnapshot() {
		if cell != nil {
			result[cell.Key] = cell.Region
		}
	}
	return result
}

/*
computeRandIndices computes the raw Rand Index and the Adjusted Rand Index
(Hubert & Arabie 1985). ARI = 1 means identical co-membership; expected random
agreement is near zero.
*/
func computeRandIndices(
	partA, partB map[string]uint8,
	sharedKeys []string,
) (float64, float64) {
	numCells := len(sharedKeys)
	if numCells < 2 {
		return 0, 0
	}

	contingency := make(map[uint8]map[uint8]int)
	rowSums := make(map[uint8]int)
	colSums := make(map[uint8]int)

	for _, key := range sharedKeys {
		groupA := partA[key]
		groupB := partB[key]
		if contingency[groupA] == nil {
			contingency[groupA] = make(map[uint8]int)
		}
		contingency[groupA][groupB]++
		rowSums[groupA]++
		colSums[groupB]++
	}

	comb2 := func(n int) float64 {
		if n < 2 {
			return 0
		}
		return float64(n*(n-1)) / 2
	}

	sumNij := 0.0
	for _, row := range contingency {
		for _, count := range row {
			sumNij += comb2(count)
		}
	}
	sumAi := 0.0
	for _, count := range rowSums {
		sumAi += comb2(count)
	}
	sumBj := 0.0
	for _, count := range colSums {
		sumBj += comb2(count)
	}

	totalPairs := comb2(numCells)
	if totalPairs == 0 {
		return 0, 0
	}

	expected := (sumAi * sumBj) / totalPairs
	maxIndex := 0.5 * (sumAi + sumBj)
	ari := 0.0
	if denominator := maxIndex - expected; denominator != 0 {
		ari = (sumNij - expected) / denominator
	}

	agreements, allPairs := 0, 0
	for first := 0; first < numCells; first++ {
		for second := first + 1; second < numCells; second++ {
			keyA := sharedKeys[first]
			keyB := sharedKeys[second]
			if (partA[keyA] == partA[keyB]) == (partB[keyA] == partB[keyB]) {
				agreements++
			}
			allPairs++
		}
	}

	rawRand := 0.0
	if allPairs > 0 {
		rawRand = float64(agreements) / float64(allPairs)
	}
	return rawRand, ari
}
