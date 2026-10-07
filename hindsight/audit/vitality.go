package audit

import (
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeVitality processes an aligned sequence of tick observations and computes Stage 1 metrics.
ticks is an ordered slice of ticks.
metricSeries maps metricName -> map[tick]value.
*/
func AnalyzeVitality(
	ticks []int64,
	metricSeries map[string]map[int64]float64,
) Stage1Vitality {
	totalTicks := len(ticks)
	if totalTicks == 0 {
		return Stage1Vitality{
			SummaryText: "No ticks observed to analyze metric vitality.",
			Passed:      false,
		}
	}

	result := Stage1Vitality{
		Metrics:        make([]MetricStat, 0, len(metricSeries)),
		RedundantPairs: make([]RedundantPair, 0),
	}

	for name, series := range metricSeries {
		count := len(series)
		coverage := float64(count) / float64(totalTicks)

		if count == 0 {
			result.DeadMetrics++
			result.Metrics = append(result.Metrics, MetricStat{
				Name:        name,
				TotalTicks:  totalTicks,
				Coverage:    0,
				IsConstant:  true,
				Status:      "DEAD",
			})
			continue
		}

		mean := 0.0
		m2 := 0.0
		minVal := math.MaxFloat64
		maxVal := -math.MaxFloat64
		zeroCount := 0
		index := 0

		for _, val := range series {
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
			result.DeadMetrics++
		}

		if !isConstant && zeroFraction == 1.0 {
			status = "ZERO"
			result.DeadMetrics++
		}

		if !isConstant && zeroFraction < 1.0 && coverage < 0.20 {
			status = "SPORADIC"
			result.SporadicMetrics++
		}

		if status == "HEALTHY" {
			result.HealthyMetrics++
		}

		result.Metrics = append(result.Metrics, MetricStat{
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

	result.TotalMetrics = len(result.Metrics)

	sort.Slice(result.Metrics, func(first, second int) bool {
		return result.Metrics[first].Name < result.Metrics[second].Name
	})

	result.RedundantPairs = findRedundantPairs(ticks, metricSeries, result.Metrics)

	result.Passed = result.HealthyMetrics > 0 && result.DeadMetrics < result.TotalMetrics/2
	result.SummaryText = fmt.Sprintf(
		"Vitality: %d/%d healthy, %d dead/constant, %d sporadic. Found %d redundant metric pairs (|r| >= 0.95).",
		result.HealthyMetrics, result.TotalMetrics, result.DeadMetrics, result.SporadicMetrics, len(result.RedundantPairs),
	)

	return result
}

/*
findRedundantPairs checks pairwise correlations among healthy metrics.
*/
func findRedundantPairs(
	ticks []int64,
	series map[string]map[int64]float64,
	stats []MetricStat,
) []RedundantPair {
	healthyNames := make([]string, 0)
	seenCanonical := make(map[string]struct{})
	for _, stat := range stats {
		if stat.Status == "HEALTHY" && stat.Coverage >= 0.20 {
			canonical := store.CellKey(stat.Name)
			if _, exists := seenCanonical[canonical]; !exists {
				seenCanonical[canonical] = struct{}{}
				healthyNames = append(healthyNames, stat.Name)
			}
		}
	}

	sort.Strings(healthyNames)
	if len(healthyNames) > 100 {
		healthyNames = healthyNames[:100]
	}

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
