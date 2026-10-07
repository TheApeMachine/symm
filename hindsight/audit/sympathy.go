package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeSympathy evaluates pairwise relationships against an empirical shuffled null.
ticks is the ordered list of observation points.
series maps metricName -> map[tick]value.
permutations is the number of shuffle iterations (e.g. 50).
*/
func AnalyzeSympathy(
	ticks []int64,
	series map[string]map[int64]float64,
	healthyMetrics []MetricStat,
	permutations int,
) Stage2Sympathy {
	seenCanonical := make(map[string]struct{})
	activeNames := make([]string, 0)

	for _, stat := range healthyMetrics {
		if stat.Status == "HEALTHY" && stat.Coverage >= 0.20 {
			canonical := store.CellKey(stat.Name)
			if _, exists := seenCanonical[canonical]; !exists {
				seenCanonical[canonical] = struct{}{}
				activeNames = append(activeNames, stat.Name)
			}
		}
	}

	sort.Strings(activeNames)

	if len(activeNames) < 2 {
		return Stage2Sympathy{
			SummaryText: "Insufficient active metrics (need at least 2) for sympathy analysis.",
			Passed:      false,
		}
	}

	// Limit to top 50 metrics to prevent O(N^2) explosion during permutation test
	if len(activeNames) > 50 {
		activeNames = activeNames[:50]
	}

	realConcordances := computeAllConcordances(ticks, series, activeNames)
	if len(realConcordances) == 0 {
		return Stage2Sympathy{
			SummaryText: "No overlapping observations found between metric pairs.",
			Passed:      false,
		}
	}

	positiveCount := 0
	inverseCount := 0
	for _, val := range realConcordances {
		if val > 0.05 {
			positiveCount++
		}
		if val < -0.05 {
			inverseCount++
		}
	}

	realMean, realStd := meanAndVar(realConcordances)
	realStd = math.Sqrt(realStd)

	realBins, realCounts := histogram(realConcordances, 25, -1.0, 1.0)

	// Permutation Null: independently shuffle metric values across ticks
	nullValues := make([]float64, 0, len(realConcordances)*permutations)
	rng := rand.New(rand.NewSource(1791))

	for iter := 0; iter < permutations; iter++ {
		shuffledSeries := make(map[string]map[int64]float64, len(activeNames))

		for _, name := range activeNames {
			vals := make([]float64, 0, len(ticks))
			for _, tick := range ticks {
				if val, ok := series[name][tick]; ok {
					vals = append(vals, val)
				}
			}

			rng.Shuffle(len(vals), func(first, second int) {
				vals[first], vals[second] = vals[second], vals[first]
			})

			shuffledMap := make(map[int64]float64, len(vals))
			valIndex := 0
			for _, tick := range ticks {
				if _, ok := series[name][tick]; ok {
					shuffledMap[tick] = vals[valIndex]
					valIndex++
				}
			}

			shuffledSeries[name] = shuffledMap
		}

		iterConcordances := computeAllConcordances(ticks, shuffledSeries, activeNames)
		nullValues = append(nullValues, iterConcordances...)
	}

	sort.Float64s(nullValues)

	nullMean, nullStd := meanAndVar(nullValues)
	nullStd = math.Sqrt(nullStd)

	p95Idx := int(float64(len(nullValues)) * 0.95)
	p99Idx := int(float64(len(nullValues)) * 0.99)
	p95 := nullValues[min(p95Idx, len(nullValues)-1)]
	p99 := nullValues[min(p99Idx, len(nullValues)-1)]

	nullBins, nullCounts := histogram(nullValues, 25, -1.0, 1.0)

	exceedingNullCount := 0
	for _, val := range realConcordances {
		if math.Abs(val) > math.Abs(p95) {
			exceedingNullCount++
		}
	}

	separationRatio := float64(exceedingNullCount) / float64(len(realConcordances))
	ksStat := computeKolmogorovSmirnov(realConcordances, nullValues)

	passed := separationRatio >= 0.15 || ksStat >= 0.20

	summary := fmt.Sprintf(
		"Sympathy: %d pairs analyzed. Real vs Shuffled Null separation ratio = %.1f%% (KS-dist = %.3f). Positive: %d, Inverse: %d.",
		len(realConcordances), separationRatio*100, ksStat, positiveCount, inverseCount,
	)

	return Stage2Sympathy{
		TotalPairs:      len(realConcordances),
		PositivePairs:   positiveCount,
		InversePairs:    inverseCount,
		RealMean:        realMean,
		RealStd:         realStd,
		RealBins:        realBins,
		RealCounts:      realCounts,
		NullDistribution: SympathyNullDistribution{
			MeanConcordance: nullMean,
			StdConcordance:  nullStd,
			Percentile95:    p95,
			Percentile99:    p99,
			HistogramBins:   nullBins,
			HistogramCounts: nullCounts,
		},
		SeparationRatio: separationRatio,
		KSStatistic:     ksStat,
		SummaryText:     summary,
		Passed:          passed,
	}
}

func computeAllConcordances(
	ticks []int64,
	series map[string]map[int64]float64,
	names []string,
) []float64 {
	results := make([]float64, 0, len(names)*(len(names)-1)/2)

	for first := 0; first < len(names); first++ {
		nameA := names[first]
		seriesA := series[nameA]

		for second := first + 1; second < len(names); second++ {
			nameB := names[second]
			seriesB := series[nameB]

			concordance, ok := pairConcordance(ticks, seriesA, seriesB)
			if ok {
				results = append(results, concordance)
			}
		}
	}

	return results
}

/*
pairConcordance calculates sign and magnitude agreement between two time series.
Positive concordance indicates moving in the same direction; negative indicates inverse movement.
*/
func pairConcordance(
	ticks []int64,
	seriesA map[int64]float64,
	seriesB map[int64]float64,
) (float64, bool) {
	alignedCount := 0
	sumProduct := 0.0
	sumA2 := 0.0
	sumB2 := 0.0

	for _, tick := range ticks {
		valA, okA := seriesA[tick]
		valB, okB := seriesB[tick]

		if okA && okB {
			alignedCount++
			sumProduct += valA * valB
			sumA2 += valA * valA
			sumB2 += valB * valB
		}
	}

	if alignedCount < 10 || sumA2 == 0 || sumB2 == 0 {
		return 0, false
	}

	denominator := math.Sqrt(sumA2 * sumB2)
	if denominator == 0 {
		return 0, false
	}

	score := sumProduct / denominator
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0, false
	}

	return score, true
}

func histogram(values []float64, bins int, minVal, maxVal float64) ([]float64, []int) {
	step := (maxVal - minVal) / float64(bins)
	binEdges := make([]float64, bins+1)
	counts := make([]int, bins)

	for idx := 0; idx <= bins; idx++ {
		binEdges[idx] = minVal + float64(idx)*step
	}

	for _, val := range values {
		if val < minVal || val > maxVal {
			continue
		}

		binIdx := int((val - minVal) / step)
		if binIdx >= bins {
			binIdx = bins - 1
		}

		counts[binIdx]++
	}

	return binEdges, counts
}

func computeKolmogorovSmirnov(realVals, nullVals []float64) float64 {
	if len(realVals) == 0 || len(nullVals) == 0 {
		return 0
	}

	sortedReal := make([]float64, len(realVals))
	copy(sortedReal, realVals)
	sort.Float64s(sortedReal)

	sortedNull := make([]float64, len(nullVals))
	copy(sortedNull, nullVals)
	sort.Float64s(sortedNull)

	maxDiff := 0.0
	realTotal := float64(len(sortedReal))
	nullTotal := float64(len(sortedNull))

	for idx, val := range sortedReal {
		realCdf := float64(idx+1) / realTotal

		nullIdx := sort.Search(len(sortedNull), func(i int) bool {
			return sortedNull[i] >= val
		})

		nullCdf := float64(nullIdx) / nullTotal
		diff := math.Abs(realCdf - nullCdf)

		if diff > maxDiff {
			maxDiff = diff
		}
	}

	return maxDiff
}
