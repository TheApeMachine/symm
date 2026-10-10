package audit

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
AnalyzeContract verifies hard mathematical domains declared by metric metadata.

The audit checks declared units as well as canonical physical invariants for known
metric families (e.g. correlation within [-1, 1], absolute correlation within [0, 1],
and variance/counts/entropy/SNR strictly non-negative).
*/
func AnalyzeContract(
	measurements []*data.Measurement,
	significance float64,
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

			unit = resolveImpliedUnit(name, unit)

			// One contract series per producer: the same label from two
			// sources is two series with two owners.
			key := meas.Source + ":" + name
			acc, exists := stats[key]
			if !exists {
				acc = &accumulator{
					unit:   unit,
					domain: declaredDomain(unit, name),
					minVal: math.MaxFloat64,
					maxVal: -math.MaxFloat64,
				}
				stats[key] = acc
			}

			acc.count++
			acc.sumVal += val

			if val < acc.minVal {
				acc.minVal = val
			}

			if val > acc.maxVal {
				acc.maxVal = val
			}

			if violation := contractViolation(unit, val, name); violation != "" {
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

	normAudit := auditMetricNormalization(measurements, significance)
	stateAudit := auditMeasurementState(measurements)

	diagnosis := "All examined metrics comply with the hard domains declared by their units."

	if len(breaches) > 0 {
		diagnosis = fmt.Sprintf(
			"CONTRACT_BREACH: %d metric series emitted values outside hard domains declared by their units. "+
				"The audit does not assign a root cause; producer mathematics/metadata must be investigated separately.",
			len(breaches),
		)
	}

	passed := len(breaches) == 0 && normAudit.Passed && stateAudit.Passed

	summaryText := fmt.Sprintf(
		"Contract Integrity: %d source:metric series evaluated (%d breached domains). Norm/Std: %d audited (%d breaches, %d z beyond the Chebyshev bound). Measurements: %d audited (WORM/State passed: %t).",
		len(stats), len(breaches), normAudit.TotalMetricsAudited, normAudit.NormalizationBreaches+normAudit.StandardizationBreaches,
		normAudit.ZMagnitudeBreaches,
		stateAudit.TotalMeasurementsAudited, stateAudit.Passed,
	)

	return Stage0Contract{
		TotalMetricsChecked:   len(stats),
		BreachingMetricsCount: len(breaches),
		TotalBreaches:         totalBreaches,
		Breaches:              breaches,
		MetricNorm:            normAudit,
		MeasurementState:      stateAudit,
		DiagnosisText:         diagnosis,
		SummaryText:           summaryText,
		Passed:                passed,
	}
}

func auditMetricNormalization(measurements []*data.Measurement, significance float64) MetricNormAudit {
	// Defined z-scores per stream (source, symbol, metric), for the
	// magnitude bound: a stream of n z-scores may not exceed zBound(n).
	type streamKey struct{ source, label, metric string }
	streamCounts := make(map[streamKey]int)

	for _, meas := range measurements {
		if meas == nil {
			continue
		}

		for entry := range meas.Read() {
			if entry != nil && entry.Metric != nil && entry.Metric.Standardizable() {
				streamCounts[streamKey{meas.Source, meas.Label, entry.Metric.Label}]++
			}
		}
	}

	zBreaches := 0
	zBreachesBySource := make(map[string]int)
	maxAbsZBySource := make(map[string]float64)

	totalAudited := 0
	normBreaches := 0
	stdBreaches := 0
	saturatedCount := 0
	sumZ := 0.0
	sumZSq := 0.0
	maxAbsZ := 0.0

	for _, meas := range measurements {
		if meas == nil {
			continue
		}

		for entry := range meas.Read() {
			if entry == nil || entry.Metric == nil {
				continue
			}

			totalAudited++
			metric := entry.Metric
			std := metric.Standardized
			norm := metric.Normalized

			if math.IsNaN(std) || math.IsInf(std, 0) {
				stdBreaches++
				continue
			}

			absZ := math.Abs(std)
			if absZ > maxAbsZ {
				maxAbsZ = absZ
			}

			if metric.Standardizable() {
				maxAbsZBySource[meas.Source] = math.Max(maxAbsZBySource[meas.Source], absZ)

				if absZ > zBound(streamCounts[streamKey{meas.Source, meas.Label, metric.Label}], significance) {
					zBreaches++
					zBreachesBySource[meas.Source]++
				}
			}

			sumZ += std
			sumZSq += std * std

			if math.IsNaN(norm) || math.IsInf(norm, 0) || norm < -1.0 || norm > 1.0 {
				normBreaches++
				continue
			}

			expectedNorm := math.Tanh(std)
			if math.Abs(norm-expectedNorm) > 1e-4 {
				normBreaches++
			}

			if math.Abs(norm) >= 0.99 {
				saturatedCount++
			}
		}
	}

	meanZ := 0.0
	varZ := 0.0
	satFraction := 0.0

	if totalAudited > 0 {
		meanZ = sumZ / float64(totalAudited)
		varZ = (sumZSq / float64(totalAudited)) - (meanZ * meanZ)
		satFraction = float64(saturatedCount) / float64(totalAudited)
	}

	summary := fmt.Sprintf(
		"Metrics Norm/Std: %d audited. Mean z=%.3f (var=%.3f, max|z|=%.3g). Saturated: %.1f%%. Breaches: norm=%d, std=%d, z beyond sqrt(n/%.2f)=%d.",
		totalAudited, meanZ, varZ, maxAbsZ, satFraction*100, normBreaches, stdBreaches, significance, zBreaches,
	)

	return MetricNormAudit{
		TotalMetricsAudited:     totalAudited,
		NormalizationBreaches:   normBreaches,
		StandardizationBreaches: stdBreaches,
		MeanZScore:              meanZ,
		VarianceZScore:          varZ,
		MaxAbsoluteZ:            maxAbsZ,
		SaturatedNormFraction:   satFraction,
		ZMagnitudeBreaches:      zBreaches,
		ZBreachesBySource:       zBreachesBySource,
		MaxAbsZBySource:         maxAbsZBySource,
		SummaryText:             summary,
		Passed:                  normBreaches == 0 && stdBreaches == 0 && zBreaches == 0,
	}
}

func auditMeasurementState(measurements []*data.Measurement) MeasurementStateAudit {
	totalAudited := 0
	unlockedBreaches := 0
	coherenceBreaches := 0
	maturityBreaches := 0
	confidenceBreaches := 0
	identityBreaches := 0

	coherences := make([]float64, 0, len(measurements))
	maturities := make([]float64, 0, len(measurements))
	confidences := make([]float64, 0, len(measurements))

	coldStartCount := 0
	settledCount := 0

	for _, meas := range measurements {
		if meas == nil {
			continue
		}

		totalAudited++

		if meas.ID == 0 {
			unlockedBreaches++
		}

		c := meas.Coherence()
		m := meas.Maturity()
		conf := meas.Confidence()

		if math.IsNaN(c) || math.IsInf(c, 0) || c < 0.0 || c > 1.0 {
			coherenceBreaches++
		}
		if !math.IsNaN(c) && !math.IsInf(c, 0) && c >= 0.0 && c <= 1.0 {
			coherences = append(coherences, c)
		}

		if math.IsNaN(m) || math.IsInf(m, 0) || m < 0.0 || m > 1.0 {
			maturityBreaches++
		}
		if !math.IsNaN(m) && !math.IsInf(m, 0) && m >= 0.0 && m <= 1.0 {
			maturities = append(maturities, m)
		}

		if math.IsNaN(conf) || math.IsInf(conf, 0) || conf < 0.0 || conf > 1.0 {
			confidenceBreaches++
		}
		if !math.IsNaN(conf) && !math.IsInf(conf, 0) && conf >= 0.0 && conf <= 1.0 {
			confidences = append(confidences, conf)
		}

		expectedConf := m * c
		if math.Abs(conf-expectedConf) > 1e-5 {
			identityBreaches++
		}

		if conf < 0.2 {
			coldStartCount++
		}
		if conf >= 0.5 {
			settledCount++
		}
	}

	meanC, medianC, p95C := distributionSummary(coherences)
	meanM, medianM, p95M := distributionSummary(maturities)
	meanConf, medianConf, p95Conf := distributionSummary(confidences)

	coldFraction := 0.0
	settledFraction := 0.0

	if totalAudited > 0 {
		coldFraction = float64(coldStartCount) / float64(totalAudited)
		settledFraction = float64(settledCount) / float64(totalAudited)
	}

	summary := fmt.Sprintf(
		"Measurements State: %d audited. Coherence mean=%.3f. Maturity mean=%.3f. Confidence mean=%.3f (settled=%.1f%%, cold=%.1f%%). Breaches: unlocked=%d, coh=%d, mat=%d, conf=%d, id=%d.",
		totalAudited, meanC, meanM, meanConf, settledFraction*100, coldFraction*100,
		unlockedBreaches, coherenceBreaches, maturityBreaches, confidenceBreaches, identityBreaches,
	)

	passed := unlockedBreaches == 0 && coherenceBreaches == 0 && maturityBreaches == 0 && confidenceBreaches == 0 && identityBreaches == 0

	return MeasurementStateAudit{
		TotalMeasurementsAudited: totalAudited,
		UnlockedBreaches:         unlockedBreaches,
		CoherenceBreaches:        coherenceBreaches,
		MaturityBreaches:         maturityBreaches,
		ConfidenceBreaches:       confidenceBreaches,
		IdentityBreaches:         identityBreaches,
		MeanCoherence:            meanC,
		MedianCoherence:          medianC,
		P95Coherence:             p95C,
		MeanMaturity:             meanM,
		MedianMaturity:           medianM,
		P95Maturity:              p95M,
		MeanConfidence:           meanConf,
		MedianConfidence:         medianConf,
		P95Confidence:            p95Conf,
		ColdStartFraction:        coldFraction,
		SettledFraction:          settledFraction,
		SummaryText:              summary,
		Passed:                   passed,
	}
}

func distributionSummary(values []float64) (float64, float64, float64) {
	if len(values) == 0 {
		return 0.0, 0.0, 0.0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	sum := 0.0
	for _, v := range sorted {
		sum += v
	}

	mean := sum / float64(len(sorted))
	median := sorted[len(sorted)/2]
	p95 := empiricalQuantile(sorted, 0.95)

	return mean, median, p95
}

func resolveImpliedUnit(name string, unit data.Unit) data.Unit {
	if unit != "" && unit != data.UnitDimensionless {
		return unit
	}

	base := name

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	if strings.HasSuffix(base, "_correlation") ||
		base == "signed_correlation" ||
		base == "absolute_correlation" ||
		base == "cohort_signed_correlation" ||
		base == "cohort_absolute_correlation" ||
		base == "correlation" {
		return data.UnitCorrelation
	}

	if strings.HasSuffix(base, "_snr") || base == "snr" {
		return data.UnitSNR
	}

	if strings.HasSuffix(base, "_entropy") || base == "entropy" {
		return data.UnitEntropy
	}

	return unit
}

/*
isAbsoluteScore reports whether a metric is the magnitude of a covariance
score, which is non-negative but unbounded above.
*/
func isAbsoluteScore(name string) bool {
	base := name

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	return strings.HasPrefix(base, "absolute_covariance_score") ||
		strings.HasPrefix(base, "cohort_absolute_covariance_score")
}

func isAbsoluteCorrelation(name string) bool {
	base := name

	if atIndex := strings.IndexByte(base, '@'); atIndex != -1 {
		base = base[:atIndex]
	}

	return strings.HasPrefix(base, "absolute_correlation") ||
		strings.HasPrefix(base, "cohort_absolute_correlation")
}

func declaredDomain(unit data.Unit, name ...string) string {
	if len(name) > 0 && isAbsoluteCorrelation(name[0]) {
		return "[0, 1]"
	}

	if len(name) > 0 && isAbsoluteScore(name[0]) {
		return "[0, +inf)"
	}

	switch unit {
	case data.UnitCorrelation:
		return "[-1, 1]"
	case data.UnitProbability, data.UnitConfidence:
		return "[0, 1]"
	case data.UnitVariance,
		data.UnitCount,
		data.UnitVolume,
		data.UnitDuration,
		data.UnitDistance,
		data.UnitEntropy,
		data.UnitSNR:
		return "[0, +inf)"
	default:
		return "finite"
	}
}

func contractViolation(unit data.Unit, value float64, name ...string) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "nan_or_inf"
	}

	if len(name) > 0 && isAbsoluteCorrelation(name[0]) {
		if value < 0.0 || value > 1.0 {
			return "domain_exceeded_[0,1]"
		}
		return ""
	}

	if len(name) > 0 && isAbsoluteScore(name[0]) {
		if value < 0.0 {
			return "negative_value_for_non_negative_unit"
		}
		return ""
	}

	switch unit {
	case data.UnitCorrelation:
		if value < -1.0 || value > 1.0 {
			return "domain_exceeded_[-1,1]"
		}
	case data.UnitProbability, data.UnitConfidence:
		if value < 0.0 || value > 1.0 {
			return "domain_exceeded_[0,1]"
		}
	case data.UnitVariance,
		data.UnitCount,
		data.UnitVolume,
		data.UnitDuration,
		data.UnitDistance,
		data.UnitEntropy,
		data.UnitSNR:
		if value < 0.0 {
			return "negative_value_for_non_negative_unit"
		}
	}

	return ""
}
