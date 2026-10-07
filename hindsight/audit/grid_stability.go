package audit

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

/*
AnalyzeGridStability develops two independent grids from disjoint chronological market segments
using production ChannelsFrom and Stream.Deform, and evaluates partition agreement (Adjusted Rand Index).
*/
func AnalyzeGridStability(
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	healthyCells []MetricStat,
) Stage3GridStability {
	totalTicks := len(ticks)
	if totalTicks < 40 {
		return Stage3GridStability{
			SummaryText: "Insufficient ticks (need at least 40) for chronological split grid stability.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	splitIdx := totalTicks / 2
	ticksA := ticks[:splitIdx]
	ticksB := ticks[splitIdx:]

	// Develop Grid A on Early period
	streamA := store.NewStream()
	gridA := store.NewGrid()
	feedGridFaithful(gridA, streamA, ticksA, tickMeasurements)
	gridA.Partition()

	// Develop Grid B on Late period
	streamB := store.NewStream()
	gridB := store.NewGrid()
	feedGridFaithful(gridB, streamB, ticksB, tickMeasurements)
	gridB.Partition()

	partStatA := evaluateGridPartition("Early Period", gridA)
	partStatB := evaluateGridPartition("Late Period", gridB)

	// Evaluate overlap and agreement on shared universe
	partitionsA := extractPartitions(gridA)
	partitionsB := extractPartitions(gridB)

	var sharedKeys []string
	for key := range partitionsA {
		if _, ok := partitionsB[key]; ok {
			sharedKeys = append(sharedKeys, key)
		}
	}

	maxUniverse := max(len(partitionsA), len(partitionsB))
	overlapFraction := 0.0
	if maxUniverse > 0 {
		overlapFraction = float64(len(sharedKeys)) / float64(maxUniverse)
	}

	randIdx, adjRandIdx := computeRandIndices(partitionsA, partitionsB, sharedKeys)

		summary := fmt.Sprintf(
		"Grid Stability: Early grid formed %d regions (%d cells); Late grid formed %d regions (%d cells). "+
			"Universe overlap = %.1f%% (%d cells). Adjusted Rand Index (ARI) = %.3f. "+
			"Note: ~5%% region sizes are structurally enforced by TargetRegionCount=20 and balancedCapacities(); "+
			"the empirical test is membership reproducibility across chronological halves.",
		partStatA.RegionCount, partStatA.CellCount,
		partStatB.RegionCount, partStatB.CellCount,
		overlapFraction*100, len(sharedKeys), adjRandIdx,
	)

	return Stage3GridStability{
		GridA:           partStatA,
		GridB:           partStatB,
		SharedUniverse:  len(sharedKeys),
		OverlapFraction: overlapFraction,
		RandIndex:       randIdx,
		AdjustedRandIdx: adjRandIdx,
		SummaryText:     summary,
		Status:          "MEASURED",
		Passed:          len(sharedKeys) >= 2,
	}
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
			rKey := fmt.Sprintf("R%d", cell.Region)
			regionSizes[rKey]++
		}
	}

	maxShare := 0.0
	for _, size := range regionSizes {
		share := float64(size) / float64(cellCount)
		if share > maxShare {
			maxShare = share
		}
	}

	isDegenerate := len(regionSizes) <= 1 || maxShare >= 0.80

	return GridPartitionStat{
		PeriodName:     name,
		CellCount:      cellCount,
		RegionCount:    len(regionSizes),
		RegionSizes:    regionSizes,
		MaxRegionShare: maxShare,
		IsDegenerate:   isDegenerate,
	}
}

func extractPartitions(grid *store.Grid) map[string]uint8 {
	result := make(map[string]uint8)
	cells := grid.CellsSnapshot()

	for _, cell := range cells {
		if cell != nil {
			result[cell.Key] = cell.Region
		}
	}

	return result
}

/*
computeRandIndices computes the raw Rand Index and the Adjusted Rand Index (Hubert & Arabie 1985).
ARI = 1 means perfect agreement; ARI ~ 0 means agreement no better than chance.
*/
func computeRandIndices(
	partA, partB map[string]uint8,
	sharedKeys []string,
) (float64, float64) {
	numCells := len(sharedKeys)
	if numCells < 2 {
		return 0, 0
	}

	// Build contingency table
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
		return float64(n*(n-1)) / 2.0
	}

	sumNij := 0.0
	for _, rowMap := range contingency {
		for _, count := range rowMap {
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

	expectedIndex := (sumAi * sumBj) / totalPairs
	maxIndex := 0.5 * (sumAi + sumBj)

	ari := 0.0
	denominator := maxIndex - expectedIndex
	if denominator > 0 {
		ari = (sumNij - expectedIndex) / denominator
	}

	agreements := 0
	allPairs := 0

	for first := 0; first < numCells; first++ {
		key1 := sharedKeys[first]

		for second := first + 1; second < numCells; second++ {
			key2 := sharedKeys[second]

			sameA := partA[key1] == partA[key2]
			sameB := partB[key1] == partB[key2]

			if sameA == sameB {
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
