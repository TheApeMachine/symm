package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridStability develops independent grids from disjoint chronological
market segments using the production ChannelsFrom -> Stream.Deform path.

It reports the largest half-vs-half comparison plus repeated comparisons using
quarter-length windows. Every observed ARI is accompanied by a random
co-membership baseline that preserves the compared partition's region-size
multiset. No ARI value is converted into a health threshold.

The ARI is NOT_A_TEST: the grid pins each metric to its region from the
label alone (store.PinRegion), so two periods sharing a cell assign it the
same region by construction and the ARI on shared cells is 1 whatever the
market did. Stationarity of the region excitation distribution is compared
with a null that reassigns ticks to the two periods at random.
*/
func AnalyzeGridStability(
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	healthyCells []MetricStat,
	permutations int,
	significance float64,
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
		"Grid mapping reproducibility: %d/%d shared cells (%.1f%% overlap), deterministic partition ARI=%.3f. "+
			"Cross-period excitation stationarity: JSD=%.3f bits, TVD=%.3f, p=%.3f vs random tick reassignment (stationary: %t). "+
			"ARI is NOT_A_TEST (partition is fixed by labels).",
		primary.sharedCells, primary.gridA.CellCount, primary.overlap*100, primary.ari,
		primary.jsd, primary.tvd, primary.jsdP, primary.jsdP > significance,
	)

	return Stage3GridStability{
		GridA:                primary.gridA,
		GridB:                primary.gridB,
		SharedUniverse:       primary.sharedCells,
		OverlapFraction:      primary.overlap,
		RandIndex:            primary.rand,
		AdjustedRandIdx:      primary.ari,
		NullAdjustedRandMean: primary.nullMean,
		DistributionJSD:      primary.jsd,
		DistributionTVD:      primary.tvd,
		StationarityPValue:   primary.jsdP,
		IsStationary:         primary.jsdP > significance,
		ARIVerdict:           VerdictNotATest,
		StabilityCurve:       curve,
		SummaryText:          summary,
		// Stationarity of the market is an observation, not a property the
		// pipeline must have, so the stage claims nothing.
		Status: VerdictMeasured,
		Passed: false,
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
	jsd         float64
	tvd         float64
	jsdP        float64
}

func compareGridWindows(
	ticksA []int64,
	ticksB []int64,
	tickMeasurements map[int64][]*data.Measurement,
	permutations int,
	seed int64,
) gridComparison {
	grid := store.NewGrid()
	partitionsA := extractPartitions(grid, ticksA, tickMeasurements)
	partitionsB := extractPartitions(grid, ticksB, tickMeasurements)
	statA := evaluateGridPartition("Period A", partitionsA)
	statB := evaluateGridPartition("Period B", partitionsB)

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

	regionsA := tickRegions(grid, ticksA, tickMeasurements)
	regionsB := tickRegions(grid, ticksB, tickMeasurements)
	jsd, tvd := computeDistributionDivergence(regionDistribution(regionsA), regionDistribution(regionsB))

	pool := append(append([][]uint8(nil), regionsA...), regionsB...)
	rng := rand.New(rand.NewSource(seed))
	nullJSD := make([]float64, 0, permutations)

	for range permutations {
		rng.Shuffle(len(pool), func(left, right int) { pool[left], pool[right] = pool[right], pool[left] })
		value, _ := computeDistributionDivergence(
			regionDistribution(pool[:len(regionsA)]), regionDistribution(pool[len(regionsA):]),
		)
		nullJSD = append(nullJSD, value)
	}

	return gridComparison{
		gridA: statA, gridB: statB,
		sharedCells: len(sharedKeys),
		overlap:     overlap,
		rand:        rawRand,
		ari:         ari,
		nullMean:    nullMean,
		jsd:         jsd,
		tvd:         tvd,
		jsdP:        upperPValue(jsd, nullJSD),
	}
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

func evaluateGridPartition(name string, partitions map[string]uint8) GridPartitionStat {
	cellCount := len(partitions)

	if cellCount == 0 {
		return GridPartitionStat{
			PeriodName:   name,
			IsDegenerate: true,
		}
	}

	regionSizes := make(map[string]int)

	for _, region := range partitions {
		regionSizes[fmt.Sprintf("R%02d", region)]++
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

func extractPartitions(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) map[string]uint8 {
	result := make(map[string]uint8)

	for _, tick := range ticks {
		for _, m := range tickMeasurements[tick] {
			if m == nil {
				continue
			}

			for entry := range m.Read() {
				if entry == nil || entry.Metric == nil {
					continue
				}

				key := fmt.Sprintf("%s:%s", m.Source, entry.Key)
				result[key] = grid.PinRegion(m.Source, entry.Key)
			}
		}
	}

	return result
}

/*
tickRegions returns, per tick, the regions the grid lit for each symbol's
frame at that tick, so distributions over any subset of ticks can be formed
without replaying the grid again.
*/
func tickRegions(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) [][]uint8 {
	result := make([][]uint8, 0, len(ticks))

	for _, tick := range ticks {
		var lit []uint8
		bySymbol := make(map[string][]*data.Measurement)

		for _, m := range tickMeasurements[tick] {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			frame := data.NewMeasurement(symMeas[0].Epoch, sym, "stability", symMeas[0].SeqIdx, tick)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()

			if reg, ok := tokenRegion(grid.Observe(frame)); ok {
				lit = append(lit, reg)
			}
		}

		result = append(result, lit)
	}

	return result
}

/*
tokenRegion parses a region token "Rnn" into its region number 1..12.
*/
func tokenRegion(token []byte) (uint8, bool) {
	if len(token) < 3 || token[0] != 'R' {
		return 0, false
	}

	reg := token[2] - '0'

	if token[1] != '0' {
		reg += 10
	}

	return reg, reg >= 1 && reg <= 12
}

/*
regionDistribution is the normalized region histogram of the given ticks.
*/
func regionDistribution(regions [][]uint8) [13]float64 {
	var counts [13]float64
	total := 0.0

	for _, lit := range regions {
		for _, reg := range lit {
			counts[reg]++
			total++
		}
	}

	if total > 0 {
		for r := 1; r <= 12; r++ {
			counts[r] /= total
		}
	}

	return counts
}

func computeDistributionDivergence(distA, distB [13]float64) (float64, float64) {
	jsd := 0.0
	tvd := 0.0

	for r := 1; r <= 12; r++ {
		p := distA[r]
		q := distB[r]
		tvd += math.Abs(p - q)

		m := 0.5 * (p + q)
		if m > 0 {
			if p > 0 {
				jsd += 0.5 * p * math.Log2(p/m)
			}
			if q > 0 {
				jsd += 0.5 * q * math.Log2(q/m)
			}
		}
	}

	tvd *= 0.5
	return jsd, tvd
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
