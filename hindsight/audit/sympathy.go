package audit

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
)

type observedVector struct {
	ticks  []int32
	values []float64
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
	significance float64,
) Stage2Sympathy {
	activeNames := make([]string, 0, len(healthyCells))
	for _, stat := range healthyCells {
		if stat.IsConstant || stat.Status == "DEAD" {
			continue
		}

		tickMap, ok := canonicalSeries[stat.Name]
		if !ok || len(tickMap) < 10 {
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

	vectors := make([]observedVector, 0, len(activeNames))
	retainedNames := make([]string, 0, len(activeNames))

	for _, name := range activeNames {
		tickMap := canonicalSeries[name]
		vecTicks := make([]int32, 0, min(len(tickMap), len(ticks)))
		vecVals := make([]float64, 0, len(vecTicks))

		for tickIndex, tick := range ticks {
			if val, ok := tickMap[tick]; ok {
				vecTicks = append(vecTicks, int32(tickIndex))
				vecVals = append(vecVals, val)
			}
		}

		if len(vecTicks) < 10 {
			continue
		}

		vectors = append(vectors, observedVector{
			ticks:  vecTicks,
			values: vecVals,
		})
		retainedNames = append(retainedNames, name)
	}

	if len(vectors) < 2 {
		return Stage2Sympathy{
			SummaryText: "Insufficient observed canonical cells for sympathy analysis.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	// Pairs are formed within one symbol (see symbolGroups).
	groups := symbolGroups(retainedNames)
	realConcordances := pairCorrelations(vectors, groups)

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
		}

		if value < 0 {
			inverseCount++
		}
	}

	realMean, realVariance := meanAndVar(realConcordances)
	realBins, realCounts := histogram(realConcordances, 25, -1, 1)

	// Precalculate empirical autocorrelation block size once per vector.
	blockSizes := make([]int, len(vectors))
	for vecIndex, vec := range vectors {
		blockSizes[vecIndex] = empiricalAutocorrelationBlockSize(vec.values)
	}

	rng := rand.New(rand.NewSource(1791))

	// Reusable permuted vector holder for the permutation iterations
	permutedVectors := make([]observedVector, len(vectors))
	for vecIndex, vec := range vectors {
		permutedVectors[vecIndex] = observedVector{
			ticks:  vec.ticks,
			values: make([]float64, len(vec.values)),
		}
	}

	nullIterations := make([][]float64, permutations)
	totalNullPairs := 0

	nullMean := 0.0
	nullM2 := 0.0
	nullCount := 0
	nullBins, nullCounts, step := createHistogramBins(25, -1, 1)

	for iter := 0; iter < permutations; iter++ {
		for vecIndex, vec := range vectors {
			permuted := blockPermute(rng, vec.values, blockSizes[vecIndex])
			permutedVectors[vecIndex].values = permuted
		}

		iterCorrs := pairCorrelations(permutedVectors, groups)
		iterAbs := make([]float64, len(iterCorrs))

		for idx, val := range iterCorrs {
			iterAbs[idx] = math.Abs(val)
			nullCount++
			delta := val - nullMean
			nullMean += delta / float64(nullCount)
			nullM2 += delta * (val - nullMean)
			addToHistogram(nullCounts, val, -1, 1, step)
		}

		nullIterations[iter] = iterAbs
		totalNullPairs += len(iterAbs)
	}

	if totalNullPairs == 0 {
		return Stage2Sympathy{
			TotalPairs:    len(realConcordances),
			PositivePairs: positiveCount,
			InversePairs:  inverseCount,
			SummaryText:   "Permutation null had no sufficiently supported cell pairs.",
			Status:        "INSUFFICIENT_DATA",
			Passed:        false,
		}
	}

	nullVariance := 0.0
	if nullCount > 1 {
		nullVariance = nullM2 / float64(nullCount-1)
	}

	absNull := make([]float64, 0, totalNullPairs)
	for _, iterAbs := range nullIterations {
		absNull = append(absNull, iterAbs...)
	}

	sort.Float64s(absNull)
	p95Abs := empiricalQuantile(absNull, 0.95)
	p99Abs := empiricalQuantile(absNull, 0.99)

	absReal := absoluteValues(realConcordances)
	sort.Float64s(absReal)

	exceeding := 0
	for _, value := range absReal {
		if value > p95Abs {
			exceeding++
		}
	}

	separationRatio := float64(exceeding) / float64(len(absReal))
	ksStat := computeKolmogorovSmirnovSorted(absReal, absNull)

	// The null for the exceedance fraction is the same fraction in each
	// shuffled iteration against the same pooled p95.
	nullFractions := make([]float64, 0, len(nullIterations))

	for _, iterAbs := range nullIterations {
		if len(iterAbs) == 0 {
			continue
		}

		above := 0
		for _, val := range iterAbs {
			if val > p95Abs {
				above++
			}
		}

		nullFractions = append(nullFractions, float64(above)/float64(len(iterAbs)))
	}

	pValue := upperPValue(separationRatio, nullFractions)
	verdict := hypothesisVerdict(pValue, len(nullFractions), significance)

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
		) + fmt.Sprintf(" Exceedance vs %d shuffled iterations: p=%.3f.", len(nullFractions), pValue),
		PValue: pValue,
		Status: verdict,
		Passed: passed(verdict),
	}
}

func pairCorrelations(vectors []observedVector, groups [][]int) []float64 {
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	type pairTask struct {
		left   int
		rights []int
	}

	totalTasks := 0
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		totalTasks += len(group) - 1
	}

	if totalTasks == 0 {
		return nil
	}

	tasks := make([]pairTask, 0, totalTasks)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		for leftAt, left := range group {
			rights := group[leftAt+1:]
			if len(rights) > 0 {
				tasks = append(tasks, pairTask{left: left, rights: rights})
			}
		}
	}

	if workers == 1 || len(tasks) < workers {
		results := make([]float64, 0)
		for _, task := range tasks {
			vecLeft := vectors[task.left]
			for _, right := range task.rights {
				if correlation, ok := maskedCorrelation(vecLeft, vectors[right]); ok {
					results = append(results, correlation)
				}
			}
		}

		return results
	}

	results := make([][]float64, workers)
	var waitGroup sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		waitGroup.Add(1)
		go func(workerID int) {
			defer waitGroup.Done()
			localResults := make([]float64, 0)

			for taskIdx := workerID; taskIdx < len(tasks); taskIdx += workers {
				task := tasks[taskIdx]
				vecLeft := vectors[task.left]

				for _, right := range task.rights {
					if correlation, ok := maskedCorrelation(vecLeft, vectors[right]); ok {
						localResults = append(localResults, correlation)
					}
				}
			}

			results[workerID] = localResults
		}(worker)
	}

	waitGroup.Wait()

	totalCorrelations := 0
	for _, chunk := range results {
		totalCorrelations += len(chunk)
	}

	merged := make([]float64, 0, totalCorrelations)
	for _, chunk := range results {
		merged = append(merged, chunk...)
	}

	return merged
}

func maskedCorrelation(left, right observedVector) (float64, bool) {
	lenLeft := len(left.ticks)
	lenRight := len(right.ticks)

	if lenLeft < 10 || lenRight < 10 {
		return 0, false
	}

	count := 0
	sumLeft, sumRight := 0.0, 0.0
	idxLeft, idxRight := 0, 0

	for idxLeft < lenLeft && idxRight < lenRight {
		tickLeft := left.ticks[idxLeft]
		tickRight := right.ticks[idxRight]

		if tickLeft == tickRight {
			count++
			sumLeft += left.values[idxLeft]
			sumRight += right.values[idxRight]
			idxLeft++
			idxRight++
			continue
		}

		if tickLeft < tickRight {
			idxLeft++
			continue
		}

		idxRight++
	}

	if count < 10 {
		return 0, false
	}

	meanLeft := sumLeft / float64(count)
	meanRight := sumRight / float64(count)
	covariance, varLeft, varRight := 0.0, 0.0, 0.0

	idxLeft, idxRight = 0, 0
	for idxLeft < lenLeft && idxRight < lenRight {
		tickLeft := left.ticks[idxLeft]
		tickRight := right.ticks[idxRight]

		if tickLeft == tickRight {
			deltaLeft := left.values[idxLeft] - meanLeft
			deltaRight := right.values[idxRight] - meanRight
			covariance += deltaLeft * deltaRight
			varLeft += deltaLeft * deltaLeft
			varRight += deltaRight * deltaRight
			idxLeft++
			idxRight++
			continue
		}

		if tickLeft < tickRight {
			idxLeft++
			continue
		}

		idxRight++
	}

	if varLeft <= 0 || varRight <= 0 {
		return 0, false
	}

	value := covariance / math.Sqrt(varLeft*varRight)
	return math.Max(-1, math.Min(1, value)), true
}

/*
empiricalAutocorrelationBlockSize calculates the empirical decorrelation horizon
as the lag where autocorrelation decays to or below 1/e (approx 0.368) or crosses zero,
grounding the block length in honest measured persistence rather than an arbitrary constant.
*/
func empiricalAutocorrelationBlockSize(values []float64) int {
	sampleCount := len(values)

	if sampleCount < 8 {
		return 2
	}

	sumVal := 0.0

	for _, val := range values {
		sumVal += val
	}

	meanVal := sumVal / float64(sampleCount)
	varianceVal := 0.0

	for _, val := range values {
		diff := val - meanVal
		varianceVal += diff * diff
	}

	if varianceVal <= 0.0 {
		return 2
	}

	maxLag := sampleCount / 4

	if maxLag < 2 {
		maxLag = 2
	}

	if maxLag > 64 {
		maxLag = 64
	}

	threshold := 1.0 / math.E

	for lag := 1; lag <= maxLag; lag++ {
		covar := 0.0

		for idx := 0; idx < sampleCount-lag; idx++ {
			covar += (values[idx] - meanVal) * (values[idx+lag] - meanVal)
		}

		autoCorr := covar / varianceVal

		if autoCorr <= threshold || autoCorr <= 0.0 {
			if lag < 2 {
				return 2
			}

			return lag
		}
	}

	return maxLag
}

func blockPermute(rng *rand.Rand, values []float64, blockSize int) []float64 {
	totalLen := len(values)

	if blockSize < 1 {
		blockSize = 1
	}

	if blockSize > totalLen {
		blockSize = totalLen
	}

	phaseShift := rng.Intn(blockSize)
	shifted := make([]float64, totalLen)

	for idx := 0; idx < totalLen; idx++ {
		shifted[idx] = values[(idx+phaseShift)%totalLen]
	}

	numBlocks := (totalLen + blockSize - 1) / blockSize
	order := make([]int, numBlocks)
	for idx := range order {
		order[idx] = idx
	}

	rng.Shuffle(numBlocks, func(idxA, idxB int) {
		order[idxA], order[idxB] = order[idxB], order[idxA]
	})

	result := make([]float64, totalLen)
	destPos := 0

	for _, blockIdx := range order {
		startPos := blockIdx * blockSize
		endPos := startPos + blockSize
		if endPos > totalLen {
			endPos = totalLen
		}

		copy(result[destPos:], shifted[startPos:endPos])
		destPos += endPos - startPos
	}

	return result
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

func computeKolmogorovSmirnovSorted(sortedReal, sortedNull []float64) float64 {
	lenReal := len(sortedReal)
	lenNull := len(sortedNull)

	if lenReal == 0 || lenNull == 0 {
		return 0
	}

	maxDistance := 0.0
	realIndex, nullIndex := 0, 0

	for realIndex < lenReal || nullIndex < lenNull {
		var currentValue float64

		if realIndex >= lenReal {
			currentValue = sortedNull[nullIndex]
		}

		if realIndex < lenReal && nullIndex >= lenNull {
			currentValue = sortedReal[realIndex]
		}

		if realIndex < lenReal && nullIndex < lenNull {
			currentValue = sortedReal[realIndex]
			if sortedNull[nullIndex] < sortedReal[realIndex] {
				currentValue = sortedNull[nullIndex]
			}
		}

		for realIndex < lenReal && sortedReal[realIndex] <= currentValue {
			realIndex++
		}

		for nullIndex < lenNull && sortedNull[nullIndex] <= currentValue {
			nullIndex++
		}

		distance := math.Abs(
			float64(realIndex)/float64(lenReal) -
				float64(nullIndex)/float64(lenNull),
		)

		if distance > maxDistance {
			maxDistance = distance
		}
	}

	return maxDistance
}

func createHistogramBins(bins int, minValue, maxValue float64) ([]float64, []int, float64) {
	if bins <= 0 {
		bins = 20
	}

	step := (maxValue - minValue) / float64(bins)
	edges := make([]float64, bins+1)
	counts := make([]int, bins)

	for index := range edges {
		edges[index] = minValue + float64(index)*step
	}

	return edges, counts, step
}

func addToHistogram(counts []int, value, minValue, maxValue, step float64) {
	if value < minValue || value > maxValue {
		return
	}

	index := int((value - minValue) / step)
	if index >= len(counts) {
		index = len(counts) - 1
	}

	counts[index]++
}

func histogram(values []float64, bins int, minValue, maxValue float64) ([]float64, []int) {
	edges, counts, step := createHistogramBins(bins, minValue, maxValue)

	for _, value := range values {
		addToHistogram(counts, value, minValue, maxValue, step)
	}

	return edges, counts
}
