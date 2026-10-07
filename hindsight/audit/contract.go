package audit

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
AnalyzeContract verifies that all observed metric values strictly adhere to their declared
mathematical domain and definedness invariants. It diagnoses out-of-bounds metrics (e.g.
correlations exceeding 1.0) without clamping them.
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

			// 1. Nan or Inf check
			if math.IsNaN(val) || math.IsInf(val, 0) {
				acc.breaches++
				acc.domain = "finite"
				acc.violation = "nan_or_inf"
				continue
			}

			// 2. Correlation domain check
			isCorrUnit := unit == data.UnitCorrelation || strings.Contains(name, "correlation")
			if isCorrUnit {
				if strings.Contains(name, "absolute") {
					acc.domain = "[0, 1]"
					if val < -1e-9 || val > 1.0+1e-6 {
						acc.breaches++
						acc.violation = "domain_exceeded_[0,1]"
					}
				} else {
					acc.domain = "[-1, 1]"
					if val < -1.0-1e-6 || val > 1.0+1e-6 {
						acc.breaches++
						acc.violation = "domain_exceeded_[-1,1]"
					}
				}
				continue
			}

			// 3. Probability / Confidence check [0, 1]
			isProb := unit == data.UnitProbability || unit == data.UnitConfidence || strings.Contains(name, "probability")
			if isProb {
				acc.domain = "[0, 1]"
				if val < -1e-9 || val > 1.0+1e-6 {
					acc.breaches++
					acc.violation = "probability_exceeded_[0,1]"
				}
				continue
			}

			// 4. Non-negative quantities: counts, variances, durations, energies
			isNonNeg := unit == data.UnitVariance || unit == data.UnitCount ||
				unit == data.UnitQuantity || unit == data.UnitVolume || unit == data.UnitNotional ||
				unit == data.UnitDuration || unit == data.UnitNanosecond ||
				strings.Contains(name, "variance") || strings.Contains(name, "count") ||
				strings.Contains(name, "energy") || strings.Contains(name, "depth")

			if isNonNeg {
				acc.domain = "[0, +inf)"
				if val < -1e-9 {
					acc.breaches++
					acc.violation = "negative_value_for_non_negative_metric"
				}
				continue
			}
		}
	}

	var breaches []ContractBreach
	totalBreaches := 0

	for name, acc := range stats {
		if acc.breaches > 0 {
			totalBreaches += acc.breaches
			meanVal := 0.0
			if acc.count > 0 {
				meanVal = acc.sumVal / float64(acc.count)
			}
			breachFrac := float64(acc.breaches) / float64(acc.count)

			breaches = append(breaches, ContractBreach{
				Metric:         name,
				DeclaredUnit:   string(acc.unit),
				DeclaredDomain: acc.domain,
				ViolationType:  acc.violation,
				BreachCount:    acc.breaches,
				TotalSamples:   acc.count,
				BreachFraction: breachFrac,
				MinVal:         acc.minVal,
				MaxVal:         acc.maxVal,
				MeanVal:        meanVal,
			})
		}
	}

	sort.Slice(breaches, func(first, second int) bool {
		if breaches[first].BreachFraction != breaches[second].BreachFraction {
			return breaches[first].BreachFraction > breaches[second].BreachFraction
		}
		return breaches[first].MaxVal > breaches[second].MaxVal
	})

	diagnosis := "All examined metrics comply with their declared mathematical domains."
	passed := len(breaches) == 0

	if len(breaches) > 0 {
		var hyBreaches []string
		for _, b := range breaches {
			if strings.Contains(b.Metric, "correlation") {
				hyBreaches = append(hyBreaches, fmt.Sprintf("%s (max=%.3f, mean=%.3f, breaches=%d/%d)", b.Metric, b.MaxVal, b.MeanVal, b.BreachCount, b.TotalSamples))
			}
		}

		if len(hyBreaches) > 0 {
			diagnosis = fmt.Sprintf(
				"CRITICAL CONTRACT BREACH: %d metrics emitted values outside mathematical bounds.\n"+
					"Top breaches include Hayashi-Yoshida correlation estimators:\n- %s\n"+
					"ROOT CAUSE DIAGNOSIS: The Hayashi-Yoshida estimator in nomagique/algo/hayashi_yoshida.go computes "+
					"cov / sqrt(leftEnergy * rightEnergy). In asynchronous high-frequency sampling with overlapping trade intervals, "+
					"single intervals on one asset overlap multiple trade intervals of peer assets. Cauchy-Schwarz does not apply "+
					"to discrete multi-overlapping intervals in finite samples without positive semi-definite (PSD) regularization, "+
					"causing the unconstrained estimator to mathematically exceed 1.0. Clamping is prohibited as it conceals the mathematical violation.",
				len(breaches),
				strings.Join(hyBreaches[:min(5, len(hyBreaches))], "\n- "),
			)
		} else {
			diagnosis = fmt.Sprintf("CONTRACT BREACH: %d metrics violated declared bounds.", len(breaches))
		}
	}

	summaryText := fmt.Sprintf(
		"Contract Integrity: %d metrics evaluated. %d metrics breached declared mathematical contracts (%d total breach events).",
		len(stats), len(breaches), totalBreaches,
	)

	return Stage0Contract{
		TotalMetricsChecked:   len(stats),
		BreachingMetricsCount: len(breaches),
		TotalBreaches:         totalBreaches,
		Breaches:              breaches,
		DiagnosisText:         diagnosis,
		SummaryText:           summaryText,
		Passed:                passed,
	}
}
