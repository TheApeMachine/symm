package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeSympathy evaluates pairwise relationships using production-faithful deformations
(produced by store.Stream.Deform) compared against an empirical shuffled null.
Both positive alignment and inverse opposition are recognized as sympathy structure.
*/
func AnalyzeSympathy(
	ticks []int64,
	canonicalSeries map[string]map[int64]float64,
	healthyCells []MetricStat,
	permutations int,
) Stage2Sympathy {
	activeNames := make([]string, 0)
	for _, stat := range healthyCells {
		if stat.Status == "HEALTHY" && stat.Coverage >= 0.20 {
			activeNames = append(activeNames, stat.Name)
		}
	}

	sort.Strings(activeNames)

	if len(activeNames) < 2 {
		return Stage2Sympathy{
			SummaryText: "Insufficient active canonical cells (need at least 2) for sympathy analysis.",
			Passed:      false,
		}
	}

	if permutations <= 0 {
		permutations = 50
	}

	// 1. Convert canonical raw measurements into zero-centered deformations using store.Stream
	stream := store.NewStream()
	deformationSeries := make(map[string]map[int64]float64, len(activeNames))
	for _, name := range activeNames {
		deformationSeries[name] = make(map[int64]float64)
	}

	for _, tick := range ticks {
		pass := make(map[string]float64, len(activeNames))
		for _, name := range activeNames {
			if val, ok := canonicalSeries[name][tick]; ok {
				pass[name] = val
			}
		}

		if len(pass) > 0 {
			deforms := stream.Deform(pass)
			for name, defVal := range deforms {
				deformationSeries[name][tick] = defVal
			}
		}
	}

	// 2. Build dense vectors across ticks
	denseVectors := make([][]float64, len(activeNames))
	for i, name := range activeNames {
		vec := make([]float64, len(ticks))
		for tIdx, tick := range ticks {
			vec[tIdx] = deformationSeries[name][tick]
		}
		denseVectors[i] = vec
	}

	realConcordances := make([]float64, 0, len(activeNames)*(len(activeNames)-1)/2)
	for i := 0; i < len(activeNames); i++ {
		for j := i + 1; j < len(activeNames); j++ {
			if corr, ok := computeVectorCorrelation(denseVectors[i], denseVectors[j]); ok {
				realConcordances = append(realConcordances, corr)
			}
		}
	}

	if len(realConcordances) == 0 {
		return Stage2Sympathy{
			SummaryText: "No overlapping observations found between canonical cell pairs.",
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

	// 3. Permutation Null: independently shuffle deformation sequences across ticks
	nullValues := make([]float64, 0, len(realConcordances)*permutations)
	rng := rand.New(rand.NewSource(1791))

	shuffled := make([][]float64, len(denseVectors))
	for i := range denseVectors {
		shuffled[i] = make([]float64, len(denseVectors[i]))
	}

	for iter := 0; iter < permutations; iter++ {
		for i := range denseVectors {
			copy(shuffled[i], denseVectors[i])
			rng.Shuffle(len(shuffled[i]), func(a, b int) {
				shuffled[i][a], shuffled[i][b] = shuffled[i][b], shuffled[i][a]
			})
		}

		for i := 0; i < len(shuffled); i++ {
			for j := i + 1; j < len(shuffled); j++ {
				if corr, ok := computeVectorCorrelation(shuffled[i], shuffled[j]); ok {
					nullValues = append(nullValues, corr)
				}
			}
		}
	}

	sort.Float64s(nullValues)

	nullMean, nullStd := meanAndVar(nullValues)
	nullStd = math.Sqrt(nullStd)

	p95Idx := int(float64(len(nullValues)) * 0.95)
	p99Idx := int(float64(len(nullValues)) * 0.99)
	p95 := nullValues[min(p95Idx, len(nullValues)-1)]
	p99 := nullValues[min(p99Idx, len(nullValues)-1)]

	nullBins, nullCounts := histogram(nullValues, 25, -1.0, 1.0)

	// Fraction of real absolute affinities that exceed null 95th percentile
	exceedingNullCount := 0
	for _, val := range realConcordances {
		if math.Abs(val) > math.Abs(p95) {
			exceedingNullCount++
		}
	}

	separationRatio := float64(exceedingNullCount) / float64(len(realConcordances))
	ksStat := computeKolmogorovSmirnov(realConcordances, nullValues)

	// Empirical pass condition: distribution separates from shuffled null
	passed := separationRatio >= 0.10 && ksStat >= 0.10

	summary := fmt.Sprintf(
		"Sympathy: %d cell pairs analyzed on deformations. Real mean = %.3f vs Shuffled Null = %.3f (p95 = %.3f). "+
			"Separation ratio = %.1f%% (KS = %.3f). Positive alignment: %d, Inverse opposition: %d.",
		len(realConcordances), realMean, nullMean, p95,
		separationRatio*100, ksStat, positiveCount, inverseCount,
	)

	return Stage2Sympathy{
		TotalPairs:       len(realConcordances),
		PositivePairs:    positiveCount,
		InversePairs:     inverseCount,
		RealMean:         realMean,
		RealStd:          realStd,
		RealBins:         realBins,
		RealCounts:       realCounts,
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

func computeVectorCorrelation(a, b []float64) (float64, bool) {
	if len(a) != len(b) || len(a) < 10 {
		return 0, false
	}

	var sumA, sumB, sumAA, sumBB, sumAB float64
	for i := 0; i < len(a); i++ {
		va := a[i]
		vb := b[i]
		sumA += va
		sumB += vb
		sumAA += va * va
		sumBB += vb * vb
		sumAB += va * vb
	}

	n := float64(len(a))
	varA := sumAA - (sumA*sumA)/n
	varB := sumBB - (sumB*sumB)/n

	if varA <= 1e-12 || varB <= 1e-12 {
		return 0, false
	}

	cov := sumAB - (sumA*sumB)/n
	denom := math.Sqrt(varA) * math.Sqrt(varB)
	if denom <= 0 {
		return 0, false
	}

	return cov / denom, true
}

func computeKolmogorovSmirnov(real, null []float64) float64 {
	if len(real) == 0 || len(null) == 0 {
		return 0
	}

	sortedReal := append([]float64(nil), real...)
	sortedNull := append([]float64(nil), null...)
	sort.Float64s(sortedReal)
	sort.Float64s(sortedNull)

	maxDist := 0.0
	realN := float64(len(sortedReal))
	nullN := float64(len(sortedNull))

	realIdx := 0
	nullIdx := 0

	for realIdx < len(sortedReal) && nullIdx < len(sortedNull) {
		valReal := sortedReal[realIdx]
		valNull := sortedNull[nullIdx]

		var currentVal float64
		if valReal <= valNull {
			currentVal = valReal
			for realIdx < len(sortedReal) && sortedReal[realIdx] == currentVal {
				realIdx++
			}
		} else {
			currentVal = valNull
			for nullIdx < len(sortedNull) && sortedNull[nullIdx] == currentVal {
				nullIdx++
			}
		}

		cdfReal := float64(realIdx) / realN
		cdfNull := float64(nullIdx) / nullN
		dist := math.Abs(cdfReal - cdfNull)
		if dist > maxDist {
			maxDist = dist
		}
	}

	return maxDist
}

func histogram(data []float64, numBins int, minEdge, maxEdge float64) ([]float64, []int) {
	if numBins <= 0 {
		numBins = 20
	}

	step := (maxEdge - minEdge) / float64(numBins)
	bins := make([]float64, numBins+1)
	counts := make([]int, numBins)

	for i := 0; i <= numBins; i++ {
		bins[i] = minEdge + float64(i)*step
	}

	for _, val := range data {
		if val < minEdge || val > maxEdge {
			continue
		}

		binIdx := int((val - minEdge) / step)
		if binIdx >= numBins {
			binIdx = numBins - 1
		}
		counts[binIdx]++
	}

	return bins, counts
}
