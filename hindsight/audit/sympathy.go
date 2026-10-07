package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/store"
)

type observedVector struct {
	values  []float64
	present []bool
}

/*
AnalyzeSympathy evaluates pairwise relationships using the production deformation
path and an empirical shuffled null.

Missing observations remain missing. Real and null pair correlations are computed
only on ticks where both channels were actually observed. The null preserves each
channel's observation mask and marginal deformation values while destroying
cross-channel temporal alignment.

Because direct and inverse movement are both sympathy, null exceedance and KS are
computed in |correlation| space.
*/
func AnalyzeSympathy(
	ticks []int64,
	canonicalSeries map[string]map[int64]float64,
	healthyCells []MetricStat,
	permutations int,
) Stage2Sympathy {
	activeNames := make([]string, 0)
	for _, stat := range healthyCells {
		if stat.IsConstant {
			continue
		}
		activeNames = append(activeNames, stat.Name)
	}
	sort.Strings(activeNames)

	if len(activeNames) < 2 || len(ticks) < 10 {
		return Stage2Sympathy{
			SummaryText: "Insufficient observed canonical cells for sympathy analysis.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}
	if permutations <= 0 {
		permutations = 50
	}

	stream := store.NewStream()
	deformationSeries := make(map[string]map[int64]float64, len(activeNames))
	activeSet := make(map[string]struct{}, len(activeNames))
	for _, name := range activeNames {
		deformationSeries[name] = make(map[int64]float64)
		activeSet[name] = struct{}{}
	}

	for _, tick := range ticks {
		pass := make(map[string]float64)
		for name, tickMap := range canonicalSeries {
			if _, active := activeSet[name]; !active {
				continue
			}
			if value, ok := tickMap[tick]; ok {
				pass[name] = value
			}
		}
		if len(pass) == 0 {
			continue
		}
		for name, value := range stream.Deform(pass) {
			deformationSeries[name][tick] = value
		}
	}

	vectors := make([]observedVector, len(activeNames))
	for index, name := range activeNames {
		vector := observedVector{
			values:  make([]float64, len(ticks)),
			present: make([]bool, len(ticks)),
		}
		for tickIndex, tick := range ticks {
			if value, ok := deformationSeries[name][tick]; ok {
				vector.values[tickIndex] = value
				vector.present[tickIndex] = true
			}
		}
		vectors[index] = vector
	}

	realConcordances := pairCorrelations(vectors)
	if len(realConcordances) == 0 {
		return Stage2Sympathy{
			SummaryText: "No canonical cell pairs had sufficient simultaneous deformation support.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	positiveCount, inverseCount := 0, 0
	for _, value := range realConcordances {
		if value > 0 {
			positiveCount++
		} else if value < 0 {
			inverseCount++
		}
	}

	realMean, realVariance := meanAndVar(realConcordances)
	realBins, realCounts := histogram(realConcordances, 25, -1, 1)

	rng := rand.New(rand.NewSource(1791))
	nullValues := make([]float64, 0, len(realConcordances)*permutations)
	shuffled := make([]observedVector, len(vectors))
	for index, vector := range vectors {
		shuffled[index] = observedVector{
			values:  make([]float64, len(vector.values)),
			present: append([]bool(nil), vector.present...),
		}
	}

	for iteration := 0; iteration < permutations; iteration++ {
		for index, vector := range vectors {
			copy(shuffled[index].values, vector.values)
			shuffleObservedValues(rng, shuffled[index].values, vector.present)
		}
		nullValues = append(nullValues, pairCorrelations(shuffled)...)
	}

	if len(nullValues) == 0 {
		return Stage2Sympathy{
			TotalPairs:    len(realConcordances),
			PositivePairs: positiveCount,
			InversePairs:  inverseCount,
			SummaryText:   "Permutation null had no sufficiently supported cell pairs.",
			Status:        "INSUFFICIENT_DATA",
			Passed:        false,
		}
	}

	nullMean, nullVariance := meanAndVar(nullValues)
	nullBins, nullCounts := histogram(nullValues, 25, -1, 1)

	absReal := absoluteValues(realConcordances)
	absNull := absoluteValues(nullValues)
	sort.Float64s(absNull)
	p95Abs := empiricalQuantile(absNull, 0.95)
	p99Abs := empiricalQuantile(absNull, 0.99)

	exceeding := 0
	for _, value := range absReal {
		if value > p95Abs {
			exceeding++
		}
	}

	separationRatio := float64(exceeding) / float64(len(absReal))
	ksStat := computeKolmogorovSmirnov(absReal, absNull)

	return Stage2Sympathy{
		TotalPairs:      len(realConcordances),
		PositivePairs:   positiveCount,
		InversePairs:    inverseCount,
		RealMean:        realMean,
		RealStd:         math.Sqrt(realVariance),
		RealBins:        realBins,
		RealCounts:      realCounts,
		SeparationRatio: separationRatio,
		KSStatistic:     ksStat,
		NullDistribution: SympathyNullDistribution{
			MeanConcordance: nullMean,
			StdConcordance:  math.Sqrt(nullVariance),
			Percentile95:    p95Abs,
			Percentile99:    p99Abs,
			HistogramBins:   nullBins,
			HistogramCounts: nullCounts,
		},
		SummaryText: fmt.Sprintf(
			"Sympathy measured on %d simultaneously-observed deformation pairs. "+
				"Real signed mean = %.3f; shuffled signed mean = %.3f; |null| p95 = %.3f; "+
				"%.1f%% of real |r| exceed |null| p95; |r| KS = %.3f. Direct: %d, inverse: %d.",
			len(realConcordances), realMean, nullMean, p95Abs,
			separationRatio*100, ksStat, positiveCount, inverseCount,
		),
		Status: "MEASURED",
		Passed: true,
	}
}

func pairCorrelations(vectors []observedVector) []float64 {
	results := make([]float64, 0, len(vectors)*(len(vectors)-1)/2)
	for left := 0; left < len(vectors); left++ {
		for right := left + 1; right < len(vectors); right++ {
			if correlation, ok := maskedCorrelation(vectors[left], vectors[right]); ok {
				results = append(results, correlation)
			}
		}
	}
	return results
}

func maskedCorrelation(left, right observedVector) (float64, bool) {
	if len(left.values) != len(right.values) ||
		len(left.present) != len(left.values) ||
		len(right.present) != len(right.values) {
		return 0, false
	}

	count := 0
	sumLeft, sumRight := 0.0, 0.0
	for index := range left.values {
		if !left.present[index] || !right.present[index] {
			continue
		}
		count++
		sumLeft += left.values[index]
		sumRight += right.values[index]
	}
	if count < 10 {
		return 0, false
	}

	meanLeft := sumLeft / float64(count)
	meanRight := sumRight / float64(count)
	covariance, varianceLeft, varianceRight := 0.0, 0.0, 0.0

	for index := range left.values {
		if !left.present[index] || !right.present[index] {
			continue
		}
		deltaLeft := left.values[index] - meanLeft
		deltaRight := right.values[index] - meanRight
		covariance += deltaLeft * deltaRight
		varianceLeft += deltaLeft * deltaLeft
		varianceRight += deltaRight * deltaRight
	}

	if varianceLeft <= 0 || varianceRight <= 0 {
		return 0, false
	}

	value := covariance / math.Sqrt(varianceLeft*varianceRight)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return math.Max(-1, math.Min(1, value)), true
}

func shuffleObservedValues(rng *rand.Rand, values []float64, present []bool) {
	indices := make([]int, 0)
	observed := make([]float64, 0)
	for index, ok := range present {
		if !ok {
			continue
		}
		indices = append(indices, index)
		observed = append(observed, values[index])
	}
	rng.Shuffle(len(observed), func(first, second int) {
		observed[first], observed[second] = observed[second], observed[first]
	})
	for index, position := range indices {
		values[position] = observed[index]
	}
}

func absoluteValues(values []float64) []float64 {
	result := make([]float64, len(values))
	for index, value := range values {
		result[index] = math.Abs(value)
	}
	return result
}

func empiricalQuantile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if quantile <= 0 {
		return sorted[0]
	}
	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func computeKolmogorovSmirnov(real, null []float64) float64 {
	if len(real) == 0 || len(null) == 0 {
		return 0
	}

	sortedReal := append([]float64(nil), real...)
	sortedNull := append([]float64(nil), null...)
	sort.Float64s(sortedReal)
	sort.Float64s(sortedNull)

	maxDistance := 0.0
	realIndex, nullIndex := 0, 0
	for realIndex < len(sortedReal) || nullIndex < len(sortedNull) {
		var value float64
		switch {
		case realIndex >= len(sortedReal):
			value = sortedNull[nullIndex]
		case nullIndex >= len(sortedNull):
			value = sortedReal[realIndex]
		case sortedReal[realIndex] <= sortedNull[nullIndex]:
			value = sortedReal[realIndex]
		default:
			value = sortedNull[nullIndex]
		}

		for realIndex < len(sortedReal) && sortedReal[realIndex] <= value {
			realIndex++
		}
		for nullIndex < len(sortedNull) && sortedNull[nullIndex] <= value {
			nullIndex++
		}

		distance := math.Abs(
			float64(realIndex)/float64(len(sortedReal)) -
				float64(nullIndex)/float64(len(sortedNull)),
		)
		if distance > maxDistance {
			maxDistance = distance
		}
	}

	return maxDistance
}

func histogram(values []float64, bins int, minValue, maxValue float64) ([]float64, []int) {
	if bins <= 0 {
		bins = 20
	}
	step := (maxValue - minValue) / float64(bins)
	edges := make([]float64, bins+1)
	counts := make([]int, bins)
	for index := range edges {
		edges[index] = minValue + float64(index)*step
	}
	for _, value := range values {
		if value < minValue || value > maxValue {
			continue
		}
		index := int((value - minValue) / step)
		if index >= bins {
			index = bins - 1
		}
		counts[index]++
	}
	return edges, counts
}
