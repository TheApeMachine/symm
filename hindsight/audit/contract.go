package audit

import (
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
AnalyzeContract verifies hard mathematical domains declared by metric metadata.

The audit deliberately does not infer domains from English substrings in metric
labels. A z-score whose label contains "correlation", for example, remains an
unbounded z-score. Tighter producer-specific contracts belong in explicit
metadata/producer validation rather than name heuristics.
*/
func AnalyzeContract(
	measurements []*data.Measurement,
) Stage0Contract {
	type accumulator struct {
		unit      data.Unit
		domain    string
		count     int
		breaches  int
		minVal    float64
		maxVal    float64
		sumVal    float64
		violation string
	}

	stats := make(map[string]*accumulator)

	for _, meas := range measurements {
		if meas == nil {
			continue
		}

		for entry := range meas.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			metric := entry.Metric
			name := metric.Label
			val := metric.Raw
			unit := metric.Unit()

			acc, exists := stats[name]
			if !exists {
				acc = &accumulator{
					unit:   unit,
					domain: declaredDomain(unit),
					minVal: math.MaxFloat64,
					maxVal: -math.MaxFloat64,
				}
				stats[name] = acc
			}

			acc.count++
			acc.sumVal += val
			if val < acc.minVal {
				acc.minVal = val
			}
			if val > acc.maxVal {
				acc.maxVal = val
			}

			if violation := contractViolation(unit, val); violation != "" {
				acc.breaches++
				acc.violation = violation
			}
		}
	}

	var breaches []ContractBreach
	totalBreaches := 0

	for name, acc := range stats {
		if acc.breaches == 0 {
			continue
		}

		totalBreaches += acc.breaches
		meanVal := 0.0
		if acc.count > 0 {
			meanVal = acc.sumVal / float64(acc.count)
		}

		breaches = append(breaches, ContractBreach{
			Metric:         name,
			DeclaredUnit:   string(acc.unit),
			DeclaredDomain: acc.domain,
			ViolationType:  acc.violation,
			BreachCount:    acc.breaches,
			TotalSamples:   acc.count,
			BreachFraction: float64(acc.breaches) / float64(acc.count),
			MinVal:         acc.minVal,
			MaxVal:         acc.maxVal,
			MeanVal:        meanVal,
		})
	}

	sort.Slice(breaches, func(first, second int) bool {
		if breaches[first].BreachFraction != breaches[second].BreachFraction {
			return breaches[first].BreachFraction > breaches[second].BreachFraction
		}
		return breaches[first].MaxVal > breaches[second].MaxVal
	})

	diagnosis := "All examined metrics comply with the hard domains declared by their units."
	if len(breaches) > 0 {
		diagnosis = fmt.Sprintf(
			"CONTRACT_BREACH: %d metric series emitted values outside hard domains declared by their units. "+
				"The audit does not assign a root cause; producer mathematics/metadata must be investigated separately.",
			len(breaches),
		)
	}

	return Stage0Contract{
		TotalMetricsChecked:   len(stats),
		BreachingMetricsCount: len(breaches),
		TotalBreaches:         totalBreaches,
		Breaches:              breaches,
		DiagnosisText:         diagnosis,
		SummaryText: fmt.Sprintf(
			"Contract Integrity: %d metrics evaluated. %d metrics breached declared unit domains (%d total breach events).",
			len(stats), len(breaches), totalBreaches,
		),
		Passed: len(breaches) == 0,
	}
}

func declaredDomain(unit data.Unit) string {
	switch unit {
	case data.UnitCorrelation:
		return "[-1, 1]"
	case data.UnitProbability, data.UnitConfidence:
		return "[0, 1]"
	case data.UnitVariance,
		data.UnitCount,
		data.UnitVolume,
		data.UnitDuration,
		data.UnitDistance:
		return "[0, +inf)"
	default:
		return "finite"
	}
}

func contractViolation(unit data.Unit, value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "nan_or_inf"
	}

	switch unit {
	case data.UnitCorrelation:
		if value < -1.0 || value > 1.0 {
			return "domain_exceeded_[-1,1]"
		}
	case data.UnitProbability, data.UnitConfidence:
		if value < 0 || value > 1.0 {
			return "domain_exceeded_[0,1]"
		}
	case data.UnitVariance,
		data.UnitCount,
		data.UnitVolume,
		data.UnitDuration,
		data.UnitDistance:
		if value < 0 {
			return "negative_value_for_non_negative_unit"
		}
	}

	return ""
}
