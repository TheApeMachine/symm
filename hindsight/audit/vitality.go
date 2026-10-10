package audit

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
)

/*
AnalyzeVitality processes both raw producer series (e.g. 7,134 peer-qualified metrics)
and canonical grid input cells (e.g. 377 cells after ChannelsFrom aggregation),
evaluating coverage, dead/sporadic series, and canonical redundancy.
*/
func AnalyzeVitality(
	ticks []int64,
	rawSeries map[string]map[int64]float64,
	canonicalSeries map[string]map[int64]float64,
) Stage1Vitality {
	totalTicks := len(ticks)
	if totalTicks == 0 {
		return Stage1Vitality{
			SummaryText: "No ticks observed to analyze metric vitality.",
			Status:      "INSUFFICIENT_DATA",
			Passed:      false,
		}
	}

	rawStats := computeMetricStats(ticks, rawSeries)
	canonicalStats := computeMetricStats(ticks, canonicalSeries)

	rawHealthy, rawDead, rawSporadic := countStatuses(rawStats)
	canonHealthy, canonDead, canonSporadic := countStatuses(canonicalStats)

	redundantPairs := findCanonicalRedundantPairs(ticks, canonicalSeries, canonicalStats)

	summary := fmt.Sprintf(
		"Vitality: Raw producers emit %d series (%d healthy, %d dead/constant, %d sporadic). "+
			"Canonical grid universe aggregates to %d cells (%d healthy, %d dead, %d sporadic). "+
			"Canonical redundancy: %d cell pairs (|r| >= 0.95).",
		len(rawStats), rawHealthy, rawDead, rawSporadic,
		len(canonicalStats), canonHealthy, canonDead, canonSporadic,
		len(redundantPairs),
	)

	return Stage1Vitality{
		RawProducerMetrics:     len(rawStats),
		RawHealthyMetrics:      rawHealthy,
		RawDeadMetrics:         rawDead,
		RawSporadicMetrics:     rawSporadic,
		RawMetrics:             rawStats,
		CanonicalGridCells:     len(canonicalStats),
		CanonicalHealthyCells:  canonHealthy,
		CanonicalDeadCells:     canonDead,
		CanonicalSporadicCells: canonSporadic,
		CanonicalCells:         canonicalStats,
		RedundantPairs:         redundantPairs,
		SummaryText:            summary,
		// Coverage and redundancy are observations (AUDIT_CONTRACT.md); this
		// stage reports them and claims nothing.
		Status: VerdictMeasured,
		Passed: false,
	}
}

func computeMetricStats(
	ticks []int64,
	series map[string]map[int64]float64,
) []MetricStat {
	totalTicks := len(ticks)
	stats := make([]MetricStat, 0, len(series))

	for name, tickMap := range series {
		count := len(tickMap)
		coverage := float64(count) / float64(totalTicks)

		if count == 0 {
			stats = append(stats, MetricStat{
				Name:       name,
				TotalTicks: totalTicks,
				Coverage:   0,
				IsConstant: true,
				Status:     "DEAD",
			})
			continue
		}

		mean := 0.0
		m2 := 0.0
		minVal := math.MaxFloat64
		maxVal := -math.MaxFloat64
		zeroCount := 0
		index := 0

		for _, val := range tickMap {
			index++
			delta := val - mean
			mean += delta / float64(index)
			m2 += delta * (val - mean)

			if val < minVal {
				minVal = val
			}
			if val > maxVal {
				maxVal = val
			}
			if val == 0 {
				zeroCount++
			}
		}

		variance := 0.0
		if count > 1 {
			variance = m2 / float64(count-1)
		}

		zeroFraction := float64(zeroCount) / float64(count)
		isConstant := (maxVal == minVal) || (variance == 0 && count > 1)

		status := "HEALTHY"
		if isConstant {
			status = "DEAD"
		}

		if !isConstant && zeroFraction == 1.0 {
			status = "ZERO"
		}

		stats = append(stats, MetricStat{
			Name:         name,
			Count:        count,
			TotalTicks:   totalTicks,
			Coverage:     coverage,
			Mean:         mean,
			Variance:     variance,
			Min:          minVal,
			Max:          maxVal,
			ZeroFraction: zeroFraction,
			IsConstant:   isConstant,
			Status:       status,
		})
	}

	sort.Slice(stats, func(first, second int) bool {
		return stats[first].Name < stats[second].Name
	})

	return stats
}

func countStatuses(stats []MetricStat) (int, int, int) {
	healthy, dead, sporadic := 0, 0, 0
	for _, s := range stats {
		switch s.Status {
		case "HEALTHY":
			healthy++
		case "DEAD", "ZERO":
			dead++
		case "SPORADIC":
			sporadic++
		}
	}
	return healthy, dead, sporadic
}

/*
findCanonicalRedundantPairs checks pairwise correlations across ALL healthy canonical grid cells.
Does not impose arbitrary ordering-dependent subsets.
*/
func findCanonicalRedundantPairs(
	ticks []int64,
	series map[string]map[int64]float64,
	stats []MetricStat,
) []RedundantPair {
	healthyNames := make([]string, 0)
	for _, stat := range stats {
		if stat.Status == "HEALTHY" {
			healthyNames = append(healthyNames, stat.Name)
		}
	}

	sort.Strings(healthyNames)

	totalHealthy := len(healthyNames)
	if totalHealthy < 2 || len(ticks) < 10 {
		return nil
	}

	// Redundancy is a question about one instrument's metrics; series of two
	// symbols are compared only by the correlation signal itself.
	groups := symbolGroups(healthyNames)

	dense := make([]denseVector, totalHealthy)
	for index, name := range healthyNames {
		ser := series[name]
		vecTicks := make([]int32, 0, min(len(ser), len(ticks)))
		vecVals := make([]float64, 0, len(vecTicks))

		for tickIndex, tick := range ticks {
			if val, ok := ser[tick]; ok {
				vecTicks = append(vecTicks, int32(tickIndex))
				vecVals = append(vecVals, val)
			}
		}

		dense[index] = denseVector{
			name:   name,
			ticks:  vecTicks,
			values: vecVals,
		}
	}

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	results := make([][]RedundantPair, workers)
	var wg sync.WaitGroup

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			local := make([]RedundantPair, 0)

			for groupIndex := workerID; groupIndex < len(groups); groupIndex += workers {
				for firstAt, first := range groups[groupIndex] {
					vecA := dense[first]

					for _, second := range groups[groupIndex][firstAt+1:] {
						vecB := dense[second]
						corr, ok := denseCorrelation(vecA.ticks, vecA.values, vecB.ticks, vecB.values)

						if !ok {
							continue
						}

						if math.Abs(corr) >= 0.95 {
							local = append(local, RedundantPair{
								MetricA:     vecA.name,
								MetricB:     vecB.name,
								Correlation: corr,
							})
						}
					}
				}
			}

			results[workerID] = local
		}(worker)
	}

	wg.Wait()

	var redundant []RedundantPair
	for _, chunk := range results {
		redundant = append(redundant, chunk...)
	}

	sort.Slice(redundant, func(i, j int) bool {
		if math.Abs(redundant[i].Correlation) != math.Abs(redundant[j].Correlation) {
			return math.Abs(redundant[i].Correlation) > math.Abs(redundant[j].Correlation)
		}

		if redundant[i].MetricA != redundant[j].MetricA {
			return redundant[i].MetricA < redundant[j].MetricA
		}

		return redundant[i].MetricB < redundant[j].MetricB
	})

	return redundant
}

type denseVector struct {
	name   string
	ticks  []int32
	values []float64
}

func denseCorrelation(
	ticksA []int32,
	valsA []float64,
	ticksB []int32,
	valsB []float64,
) (float64, bool) {
	lenA := len(ticksA)
	lenB := len(ticksB)

	if lenA < 10 || lenB < 10 {
		return 0, false
	}

	count := 0
	sumA := 0.0
	sumB := 0.0
	idxA, idxB := 0, 0

	for idxA < lenA && idxB < lenB {
		tickA := ticksA[idxA]
		tickB := ticksB[idxB]

		if tickA == tickB {
			count++
			sumA += valsA[idxA]
			sumB += valsB[idxB]
			idxA++
			idxB++
			continue
		}

		if tickA < tickB {
			idxA++
			continue
		}

		idxB++
	}

	if count < 10 {
		return 0, false
	}

	meanA := sumA / float64(count)
	meanB := sumB / float64(count)

	m2A := 0.0
	m2B := 0.0
	cov := 0.0

	idxA, idxB = 0, 0
	for idxA < lenA && idxB < lenB {
		tickA := ticksA[idxA]
		tickB := ticksB[idxB]

		if tickA == tickB {
			diffA := valsA[idxA] - meanA
			diffB := valsB[idxB] - meanB
			m2A += diffA * diffA
			m2B += diffB * diffB
			cov += diffA * diffB
			idxA++
			idxB++
			continue
		}

		if tickA < tickB {
			idxA++
			continue
		}

		idxB++
	}

	if m2A <= 0 || m2B <= 0 {
		return 0, false
	}

	corr := cov / (math.Sqrt(m2A) * math.Sqrt(m2B))
	return corr, true
}

func meanAndVar(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	mean := 0.0
	m2 := 0.0

	for idx, val := range values {
		delta := val - mean
		mean += delta / float64(idx+1)
		m2 += delta * (val - mean)
	}

	variance := 0.0
	if len(values) > 1 {
		variance = m2 / float64(len(values)-1)
	}

	return mean, variance
}

/*
seriesKey names one stored series: a producer's metric for one symbol.
Series of different symbols never share a key, so a tick that carries two
symbols' values keeps both rather than the last one written.
*/
func seriesKey(source, symbol, metric string) string {
	return source + "|" + symbol + "|" + metric
}

/*
seriesSymbol returns the symbol of a seriesKey, or "" for any other name.
*/
func seriesSymbol(key string) string {
	first := strings.IndexByte(key, '|')

	if first == -1 {
		return ""
	}

	rest := key[first+1:]
	second := strings.IndexByte(rest, '|')

	if second == -1 {
		return ""
	}

	return rest[:second]
}

/*
symbolGroups partitions the indexes of names by seriesSymbol, in order.
*/
func symbolGroups(names []string) [][]int {
	index := make(map[string]int)
	var groups [][]int

	for position, name := range names {
		symbol := seriesSymbol(name)
		group, found := index[symbol]

		if !found {
			group = len(groups)
			index[symbol] = group
			groups = append(groups, nil)
		}

		groups[group] = append(groups[group], position)
	}

	return groups
}
