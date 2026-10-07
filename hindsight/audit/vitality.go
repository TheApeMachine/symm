package audit

import (
	"fmt"
	"math"
	"sort"
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
			Passed:      false,
		}
	}

	rawStats := computeMetricStats(ticks, rawSeries)
	canonicalStats := computeMetricStats(ticks, canonicalSeries)

	rawHealthy, rawDead, rawSporadic := countStatuses(rawStats)
	canonHealthy, canonDead, canonSporadic := countStatuses(canonicalStats)

	redundantPairs := findCanonicalRedundantPairs(ticks, canonicalSeries, canonicalStats)

	passed := canonHealthy > 0 && canonDead < len(canonicalStats)/2

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
		Passed:                 passed,
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

		if !isConstant && zeroFraction < 1.0 && coverage < 0.20 {
			status = "SPORADIC"
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
		if stat.Status == "HEALTHY" && stat.Coverage >= 0.20 {
			healthyNames = append(healthyNames, stat.Name)
		}
	}

	sort.Strings(healthyNames)

	redundant := make([]RedundantPair, 0)
	totalHealthy := len(healthyNames)

	for first := 0; first < totalHealthy; first++ {
		nameA := healthyNames[first]
		seriesA := series[nameA]

		for second := first + 1; second < totalHealthy; second++ {
			nameB := healthyNames[second]
			seriesB := series[nameB]

			corr, ok := computeCorrelation(ticks, seriesA, seriesB)
			if !ok {
				continue
			}

			if math.Abs(corr) >= 0.95 {
				redundant = append(redundant, RedundantPair{
					MetricA:     nameA,
					MetricB:     nameB,
					Correlation: corr,
				})
			}
		}
	}

	return redundant
}

func computeCorrelation(
	ticks []int64,
	seriesA map[int64]float64,
	seriesB map[int64]float64,
) (float64, bool) {
	var valsA []float64
	var valsB []float64

	for _, tick := range ticks {
		valA, okA := seriesA[tick]
		valB, okB := seriesB[tick]

		if okA && okB {
			valsA = append(valsA, valA)
			valsB = append(valsB, valB)
		}
	}

	if len(valsA) < 10 {
		return 0, false
	}

	meanA, varA := meanAndVar(valsA)
	meanB, varB := meanAndVar(valsB)

	if varA == 0 || varB == 0 {
		return 0, false
	}

	cov := 0.0
	for idx := range valsA {
		cov += (valsA[idx] - meanA) * (valsB[idx] - meanB)
	}

	cov /= float64(len(valsA) - 1)
	corr := cov / (math.Sqrt(varA) * math.Sqrt(varB))

	if math.IsNaN(corr) || math.IsInf(corr, 0) {
		return 0, false
	}

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
