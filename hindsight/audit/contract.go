package audit

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
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
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.contract] nil measurement in audit slice",
				nil,
			))

			continue
		}

		if meas.ID == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.contract] unfinalized measurement (ID == 0)",
				nil,
			), "epoch", meas.Epoch, "label", meas.Label, "source", meas.Source)
		}

		for entry := range meas.Read() {
			if entry == nil || entry.Metric == nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"[audit.contract] nil metric entry in measurement",
					nil,
				), "label", meas.Label, "source", meas.Source)

				continue
			}

			metric := entry.Metric
			name := metric.Label()
			val := metric.Raw()
			unit := metric.Unit()

			if unit == "" || unit == data.UnitDimensionless {
				if canonicalUnit, _ := data.GetDimension(meas.Source, name); canonicalUnit != data.UnitNone {
					unit = canonicalUnit
				}
			}

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

	for _, b := range breaches {
		errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("[audit.contract] metric contract violation: %s", b.ViolationType),
			nil,
		), "metric", b.Metric, "unit", b.DeclaredUnit, "domain", b.DeclaredDomain, "breaches", b.BreachCount, "total", b.TotalSamples, "min", b.MinVal, "max", b.MaxVal)
	}

	normAudit := auditMetricNormalization(measurements)
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
		"Contract Integrity: %d source:metric series evaluated (%d breached domains). Norm/Std: %d audited (%d mature, %d breaches [%d unstandardized mature], %d z beyond the Chebyshev bound). Measurements: %d audited (WORM/State passed: %t).",
		len(stats), len(breaches), normAudit.TotalMetricsAudited, normAudit.MatureMetricsAudited,
		normAudit.NormalizationBreaches+normAudit.StandardizationBreaches, normAudit.UnstandardizedMatureBreaches,
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

func auditMetricNormalization(measurements []*data.Measurement) MetricNormAudit {
	zBreaches := 0
	zBreachesBySource := make(map[string]int)
	maxAbsZBySource := make(map[string]float64)

	totalAudited := 0
	matureAudited := 0
	unstandardizedMatureBreaches := 0
	normBreaches := 0
	stdBreaches := 0
	saturatedCount := 0
	sumZ := 0.0
	sumZSq := 0.0
	maxAbsZ := 0.0

	streamCounts := make(map[string]int)

	for _, meas := range measurements {
		if meas == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.metric_norm] nil measurement encountered in audit stream",
				nil,
			))

			continue
		}

		if meas.ID == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.metric_norm] measurement with unassigned ID (zero) encountered in audit stream",
				nil,
			), "source", meas.Source, "label", meas.Label, "epoch", meas.Epoch)
		}

		for entry := range meas.Read() {
			if entry == nil || entry.Metric == nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"[audit.metric_norm] nil metric entry in measurement",
					nil,
				), "label", meas.Label, "source", meas.Source)

				continue
			}

			totalAudited++
			metric := entry.Metric
			snm := metric.SNM()

			streamKey := meas.Source + "|" + meas.Label + "|" + entry.Key
			streamCounts[streamKey]++
			streamCount := streamCounts[streamKey]

			if streamCount > int(core.MinimumRankPrior) {
				matureAudited++
			}

			if math.IsNaN(snm) || math.IsInf(snm, 0) || snm < -1.0 || snm > 1.0 {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"[audit.metric_norm] SNM value out of bounds [-1, 1] or NaN/Inf",
					nil,
				), "source", meas.Source, "label", meas.Label, "metric", metric.Label(), "snm", snm)

				normBreaches++
				stdBreaches++
				continue
			}

			absSNM := math.Abs(snm)

			if absSNM > maxAbsZ {
				maxAbsZ = absSNM
			}

			maxAbsZBySource[meas.Source] = math.Max(maxAbsZBySource[meas.Source], absSNM)

			sumZ += snm
			sumZSq += snm * snm

			if absSNM >= 0.99 {
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
		"Metrics Norm/Std: %d audited (%d mature). Mean z=%.3f (var=%.3f, max|z|=%.3g). Saturated: %.1f%%. Breaches: norm=%d, std=%d (unstandardized mature=%d), z beyond rank bound=%d.",
		totalAudited, matureAudited, meanZ, varZ, maxAbsZ, satFraction*100, normBreaches, stdBreaches, unstandardizedMatureBreaches, zBreaches,
	)

	return MetricNormAudit{
		TotalMetricsAudited:          totalAudited,
		MatureMetricsAudited:         matureAudited,
		UnstandardizedMatureBreaches: unstandardizedMatureBreaches,
		NormalizationBreaches:        normBreaches,
		StandardizationBreaches:      stdBreaches,
		MeanZScore:                   meanZ,
		VarianceZScore:               varZ,
		MaxAbsoluteZ:                 maxAbsZ,
		SaturatedNormFraction:        satFraction,
		ZMagnitudeBreaches:           zBreaches,
		ZBreachesBySource:            zBreachesBySource,
		MaxAbsZBySource:              maxAbsZBySource,
		SummaryText:                  summary,
		Passed:                       normBreaches == 0 && stdBreaches == 0 && zBreaches == 0,
	}
}

func auditMeasurementState(measurements []*data.Measurement) MeasurementStateAudit {
	totalAudited := 0
	unlockedBreaches := 0
	epochBreaches := 0
	labelBreaches := 0
	sourceBreaches := 0
	timingBreaches := 0
	maturityBreaches := 0
	energyBreaches := 0
	snmBreaches := 0
	dampenBreaches := 0

	maturities := make([]float64, 0, len(measurements))

	coldStartCount := 0
	settledCount := 0

	for _, meas := range measurements {
		if meas == nil {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[audit] measurement is nil",
				nil,
			))

			continue
		}

		totalAudited++

		if meas.ID == 0 {
			unlockedBreaches++
		}

		if meas.Epoch <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] measurement epoch <= 0",
				nil,
			), "epoch", meas.Epoch, "label", meas.Label, "source", meas.Source)

			epochBreaches++
		}

		if meas.Label == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] measurement label is empty",
				nil,
			), "source", meas.Source)

			labelBreaches++
		}

		if meas.Source == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] measurement source is empty",
				nil,
			), "label", meas.Label)

			sourceBreaches++
		}

		if meas.At.IsZero() {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] measurement timestamp At is zero",
				nil,
			), "label", meas.Label, "source", meas.Source)

			timingBreaches++
		}

		if meas.ID == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] measurement is not finalized (ID == 0)",
				nil,
			), "epoch", meas.Epoch, "label", meas.Label, "source", meas.Source)
		}

		maturity := 1.0

		var sumAbs float64
		var sumSq float64
		metricCount := 0

		for entry := range meas.Read() {
			if entry != nil && entry.Metric != nil {
				metricCount++
				stdVal := entry.Metric.SNM()

				if math.IsNaN(stdVal) || math.IsInf(stdVal, 0) || stdVal < -1.0 || stdVal > 1.0 {
					errnie.Error(errnie.Err(
						errnie.Validation,
						"[audit.measurement_state] SNM out of bounds [-1, 1]",
						nil,
					), "snm", stdVal, "label", meas.Label, "source", meas.Source, "metric", entry.Key)

					snmBreaches++
				}

				sumAbs += math.Abs(stdVal)
				sumSq += stdVal * stdVal
			}
		}

		energy := sumSq

		if math.IsNaN(energy) || math.IsInf(energy, 0) || energy < 0.0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] energy out of bounds [0, +inf)",
				nil,
			), "energy", energy, "label", meas.Label, "source", meas.Source)

			energyBreaches++
		}

		if math.IsNaN(maturity) || math.IsInf(maturity, 0) || maturity < -1e-9 || maturity > 1.0+1e-9 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.measurement_state] maturity out of bounds [0, 1]",
				nil,
			), "maturity", maturity, "label", meas.Label, "source", meas.Source)

			maturityBreaches++
		}

		if !math.IsNaN(maturity) && !math.IsInf(maturity, 0) && maturity >= -1e-9 && maturity <= 1.0+1e-9 {
			maturities = append(maturities, maturity)
		}

		if maturity < 0.2 {
			coldStartCount++
		}

		if maturity >= 0.5 {
			settledCount++
		}
	}

	meanM, medianM, p95M := distributionSummary(maturities)

	coldFraction := 0.0
	settledFraction := 0.0

	if totalAudited > 0 {
		coldFraction = float64(coldStartCount) / float64(totalAudited)
		settledFraction = float64(settledCount) / float64(totalAudited)
	}

	summary := fmt.Sprintf(
		"Measurements State: %d audited. Maturity mean=%.3f (settled=%.1f%%, cold=%.1f%%). Breaches: unlocked=%d, epoch=%d, label=%d, source=%d, timing=%d, mat=%d, energy=%d, snm=%d, dampen=%d.",
		totalAudited, meanM, settledFraction*100, coldFraction*100,
		unlockedBreaches, epochBreaches, labelBreaches, sourceBreaches, timingBreaches, maturityBreaches, energyBreaches, snmBreaches, dampenBreaches,
	)

	passed := unlockedBreaches == 0 && epochBreaches == 0 && labelBreaches == 0 &&
		sourceBreaches == 0 && timingBreaches == 0 && maturityBreaches == 0 &&
		energyBreaches == 0 && snmBreaches == 0 && dampenBreaches == 0

	return MeasurementStateAudit{
		TotalMeasurementsAudited: totalAudited,
		UnlockedBreaches:         unlockedBreaches,
		EpochBreaches:            epochBreaches,
		LabelBreaches:            labelBreaches,
		SourceBreaches:           sourceBreaches,
		TimingBreaches:           timingBreaches,
		MaturityBreaches:         maturityBreaches,
		EnergyBreaches:           energyBreaches,
		SNMBreaches:              snmBreaches,
		DampenBreaches:           dampenBreaches,
		MeanMaturity:             meanM,
		MedianMaturity:           medianM,
		P95Maturity:              p95M,
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
	for _, val := range sorted {
		sum += val
	}

	mean := sum / float64(len(sorted))
	median := sorted[len(sorted)/2]
	p95 := empiricalQuantile(sorted, 0.95)

	return mean, median, p95
}

/*
isAbsoluteScore reports whether a covariance score metric is explicitly an
absolute score (its magnitude), which is non-negative [0, +inf).
*/
func isAbsoluteScore(name string) bool {
	return strings.HasPrefix(name, "absolute_covariance_score") ||
		strings.HasPrefix(name, "cohort_absolute_covariance_score")
}

func declaredDomain(unit data.Unit, name ...string) string {
	metricName := ""

	if len(name) > 0 {
		metricName = name[0]
	}

	switch unit {
	case data.UnitProbability, data.UnitConfidence:
		return "[0, 1]"

	case data.UnitCorrelation, data.UnitImbalance:
		return "[-1, 1]"

	case data.UnitPrice,
		data.UnitCurrency,
		data.UnitQuantity,
		data.UnitVolume,
		data.UnitNotional,
		data.UnitSpread,
		data.UnitRelativeSpread,
		data.UnitQuoteCurrencyPerBaseUnit,
		data.UnitBaseCurrency,
		data.UnitQuoteCurrency,
		data.UnitCount,
		data.UnitVariance,
		data.UnitStandardDeviation,
		data.UnitSNR,
		data.UnitEntropy,
		data.UnitDistance,
		data.UnitRate,
		data.UnitPerSecond,
		data.UnitTradeRate,
		data.UnitVolumeRate,
		data.UnitNotionalRate,
		data.UnitRatio,
		data.UnitPercent,
		data.UnitBasisPoints,
		data.UnitDuration,
		data.UnitNanosecond,
		data.UnitMicrosecond,
		data.UnitMillisecond,
		data.UnitSecond,
		data.UnitMinute,
		data.UnitHour:
		return "[0, +inf)"

	case data.UnitCovarianceScore:
		if isAbsoluteScore(metricName) {
			return "[0, +inf)"
		}

		return "finite"

	case data.UnitZScore,
		data.UnitLogReturn,
		data.UnitVelocity,
		data.UnitAcceleration,
		data.UnitCovariance,
		data.UnitCountResidual,
		data.UnitNat,
		data.UnitPriceImpact,
		data.UnitDivergence,
		data.UnitSignedNotional,
		data.UnitSignedQuantity,
		data.UnitSignedRate,
		data.UnitSignedNotionalRate,
		data.UnitTimeOffset:
		return "finite"

	default:
		return "undeclared"
	}
}

func contractViolation(unit data.Unit, value float64, name ...string) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "nan_or_inf"
	}

	metricName := ""

	if len(name) > 0 {
		metricName = name[0]
	}

	switch unit {
	case data.UnitProbability, data.UnitConfidence:
		if value < 0.0 || value > 1.0 {
			return "domain_exceeded_[0,1]"
		}

		return ""

	case data.UnitCorrelation, data.UnitImbalance:
		if value < -1.0 || value > 1.0 {
			return "domain_exceeded_[-1,1]"
		}

		return ""

	case data.UnitPrice,
		data.UnitCurrency,
		data.UnitQuantity,
		data.UnitVolume,
		data.UnitNotional,
		data.UnitSpread,
		data.UnitRelativeSpread,
		data.UnitQuoteCurrencyPerBaseUnit,
		data.UnitBaseCurrency,
		data.UnitQuoteCurrency,
		data.UnitCount,
		data.UnitVariance,
		data.UnitStandardDeviation,
		data.UnitSNR,
		data.UnitEntropy,
		data.UnitDistance,
		data.UnitRate,
		data.UnitPerSecond,
		data.UnitTradeRate,
		data.UnitVolumeRate,
		data.UnitNotionalRate,
		data.UnitRatio,
		data.UnitPercent,
		data.UnitBasisPoints,
		data.UnitDuration,
		data.UnitNanosecond,
		data.UnitMicrosecond,
		data.UnitMillisecond,
		data.UnitSecond,
		data.UnitMinute,
		data.UnitHour:
		if value < 0.0 {
			return "negative_value_for_non_negative_unit"
		}

		return ""

	case data.UnitCovarianceScore:
		if isAbsoluteScore(metricName) {
			if value < 0.0 {
				return "negative_value_for_non_negative_unit"
			}
		}

		return ""

	case data.UnitZScore,
		data.UnitLogReturn,
		data.UnitVelocity,
		data.UnitAcceleration,
		data.UnitCovariance,
		data.UnitCountResidual,
		data.UnitNat,
		data.UnitPriceImpact,
		data.UnitDivergence,
		data.UnitSignedNotional,
		data.UnitSignedQuantity,
		data.UnitSignedRate,
		data.UnitSignedNotionalRate,
		data.UnitTimeOffset:
		return ""

	default:
		return "undeclared_or_unknown_physical_dimension"
	}
}
