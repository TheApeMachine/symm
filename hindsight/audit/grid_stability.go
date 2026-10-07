package audit

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridStability develops two independent grids from disjoint chronological segments
and evaluates partition agreement (Adjusted Rand Index) and region balance.
*/
func AnalyzeGridStability(
	ticks []int64,
	series map[string]map[int64]float64,
	healthyMetrics []MetricStat,
) Stage3GridStability {
	totalTicks := len(ticks)
	if totalTicks < 40 {
		return Stage3GridStability{
			SummaryText: "Insufficient ticks (need at least 40) for chronological split grid stability.",
			Passed:      false,
		}
	}

	splitIdx := totalTicks / 2
	ticksA := ticks[:splitIdx]
	ticksB := ticks[splitIdx:]

	gridA := store.NewGrid()
	feedGrid(gridA, ticksA, series)
	gridA.Partition()

	gridB := store.NewGrid()
	feedGrid(gridB, ticksB, series)
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

	passed := !partStatA.IsDegenerate && !partStatB.IsDegenerate && overlapFraction >= 0.50 && adjRandIdx >= 0.30

	summary := fmt.Sprintf(
		"Grid Stability: Early grid formed %d regions (%d cells, max share %.1f%%); Late grid formed %d regions (%d cells, max share %.1f%%). Universe overlap = %.1f%%. Adjusted Rand Index (ARI) = %.3f.",
		partStatA.RegionCount, partStatA.CellCount, partStatA.MaxRegionShare*100,
		partStatB.RegionCount, partStatB.CellCount, partStatB.MaxRegionShare*100,
		overlapFraction*100, adjRandIdx,
	)

	return Stage3GridStability{
		GridA:           partStatA,
		GridB:           partStatB,
		SharedUniverse:  len(sharedKeys),
		OverlapFraction: overlapFraction,
		RandIndex:       randIdx,
		AdjustedRandIdx: adjRandIdx,
		SummaryText:     summary,
		Passed:          passed,
	}
}

func feedGrid(
	grid *store.Grid,
	ticks []int64,
	series map[string]map[int64]float64,
) {
	for _, tick := range ticks {
		deformations := make(map[string]float64)

		for name, tickMap := range series {
			if val, ok := tickMap[tick]; ok {
				deformations[store.CellKey(name)] = val
			}
		}

		if len(deformations) > 0 {
			grid.Update(tick, deformations)
		}
	}
}

func evaluateGridPartition(name string, grid *store.Grid) GridPartitionStat {
	cells := grid.CellsSnapshot()
	cellCount := len(cells)
	regionSizes := make(map[string]int)

	for _, cell := range cells {
		if cell == nil {
			continue
		}

		regStr := fmt.Sprintf("Region_%d", cell.Region)
		regionSizes[regStr]++
	}

	maxSize := 0
	for _, size := range regionSizes {
		if size > maxSize {
			maxSize = size
		}
	}

	maxShare := 0.0
	if cellCount > 0 {
		maxShare = float64(maxSize) / float64(cellCount)
	}

	isDegenerate := maxShare >= 0.70 || len(regionSizes) < 2

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

	// Disagreements for raw Rand Index
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

	if math.IsNaN(ari) || math.IsInf(ari, 0) {
		ari = 0
	}

	return rawRand, ari
}
