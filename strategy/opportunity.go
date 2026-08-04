package strategy

import (
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

func highVelocityOpportunity(thesis *types.Thesis, symbol string) bool {
	if thesis == nil || len(thesis.Categories) == 0 {
		return false
	}

	categories := thesis.Categories[symbol]
	highVelocity := false

	for _, category := range categories {
		switch category.Type {
		case types.CategoryExhaustion,
			types.CategoryFadedExhaustion,
			types.CategoryThermalExhaustion,
			types.CategoryMechanicalCollapse,
			types.CategoryActiveReversal,
			types.CategorySpoofTrap,
			types.CategoryToxicBluff,
			types.CategoryBookThinning:
			return false
		case types.CategoryVerticalIgnition,
			types.CategoryFrenzy,
			types.CategoryAggressiveDrive,
			types.CategoryLiquidityShock,
			types.CategoryLoadedImbalance:
			highVelocity = true
		}
	}

	return highVelocity
}

func getMetricValue(measurement *types.Measurement, metricKey string) float64 {
	if measurement == nil || measurement.Metrics == nil {
		return 0
	}

	sample, ok := measurement.Metrics[metricKey]

	if !ok {
		return 0
	}

	if sample.Normalized != nil && !math.IsNaN(*sample.Normalized) && !math.IsInf(*sample.Normalized, 0) {
		return *sample.Normalized
	}

	if !math.IsNaN(sample.Raw) && !math.IsInf(sample.Raw, 0) {
		return sample.Raw
	}

	return 0
}

func momentumTerms(thesis *types.Thesis, symbol string) (boost float64, survival float64) {
	if thesis == nil {
		return 1.0, 1.0
	}

	hawkesFactor := 1.0
	pumpFactor := 1.0
	flowFactor := 1.0
	thinFactor := 1.0
	sentimentFactor := 1.0
	exhaustionPenalty := 0.0

	if thesis.Measurements != nil {
		thesis.Measurements.Range(func(key, value any) bool {
			rows, ok := value.([]*types.Measurement)

			if !ok {
				measurement, measurementOK := value.(*types.Measurement)

				if !measurementOK || measurement == nil {
					return true
				}

				rows = []*types.Measurement{measurement}
			}

			for _, measurement := range rows {
				if measurement == nil || measurement.Symbol != symbol {
					continue
				}

				switch measurement.Source {
				case types.SourcePumpDump:
					ignition := getMetricValue(measurement, string(types.MetricKey(types.MetricIgnition, types.SideNone)))
					trend := getMetricValue(measurement, string(types.MetricKey(types.MetricTrend, types.SideNone)))
					rvol := getMetricValue(measurement, string(types.MetricKey(types.MetricRVOL, types.SideNone)))
					compression := getMetricValue(measurement, string(types.MetricKey(types.MetricCompression, types.SideNone)))
					exhaustion := getMetricValue(measurement, string(types.MetricKey(types.MetricExhaustion, types.SideNone)))

					rvolTerm := 0.0

					if rvol > 1.0 {
						rvolTerm = math.Log(rvol)
					}

					pumpVelocity := 1.0 + ignition*(1.0+rvolTerm) + trend + compression

					if pumpVelocity > pumpFactor {
						pumpFactor = pumpVelocity
					}

					if exhaustion > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, exhaustion)
					}

				case types.SourceHawkes:
					descendants := getMetricValue(measurement, string(types.MetricKey(types.MetricTotalDescendants, types.SideBuy)))
					offspring := getMetricValue(measurement, string(types.MetricKey(types.MetricImmediateOffspring, types.SideBuy)))
					intensity := getMetricValue(measurement, string(types.MetricKey(types.MetricConditionalIntensity, types.SideBuy)))
					baseline := getMetricValue(measurement, string(types.MetricKey(types.MetricBaselineIntensity, types.SideBuy)))

					hawkesCascade := 1.0

					if descendants > 1.0 {
						hawkesCascade = descendants
					}

					if descendants <= 1.0 && offspring > 0 && offspring < 1.0 {
						hawkesCascade = 1.0 / (1.0 - offspring)
					}

					if baseline > 0 && intensity > baseline {
						hawkesCascade *= (intensity / baseline)
					}

					if hawkesCascade > hawkesFactor {
						hawkesFactor = hawkesCascade
					}

				case types.SourceCVD:
					drive := getMetricValue(measurement, string(types.MetricKey(types.MetricDrive, types.SideBuy)))
					absorption := getMetricValue(measurement, string(types.MetricKey(types.MetricAbsorption, types.SideNone)))

					flowFactor = math.Max(flowFactor, 1.0+drive)

					if absorption > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, absorption*0.5)
					}

				case types.SourceSentiment:
					surge := getMetricValue(measurement, string(types.MetricKey(types.MetricSurgeScore, types.SideNone)))
					lead := getMetricValue(measurement, string(types.MetricKey(types.MetricLeaderStrength, types.SideNone)))

					if surge > 0 || lead > 0 {
						sentimentFactor = math.Max(sentimentFactor, 1.0+surge+lead)
					}

				case types.SourceDepthFlow:
					thin := getMetricValue(measurement, string(types.MetricKey(types.MetricThinScore, types.SideNone)))
					spoof := getMetricValue(measurement, string(types.MetricKey(types.MetricSpoofScore, types.SideNone)))

					if thin > 0 {
						thinFactor = math.Max(thinFactor, 1.0+thin)
					}

					if spoof > 0 {
						exhaustionPenalty = math.Max(exhaustionPenalty, spoof)
					}

				case types.SourceLiquidity:
					scarcity := getMetricValue(measurement, string(types.MetricKey(types.MetricScarcityScore, types.SideNone)))

					if scarcity > 0 {
						thinFactor = math.Max(thinFactor, 1.0+scarcity)
					}
				}
			}

			return true
		})
	}

	categoryFactor := 1.0

	if len(thesis.Categories) > 0 {
		for _, category := range thesis.Categories[symbol] {
			boostMetric := category.Strength * category.Confidence * (1.0 - math.Min(1.0, category.Surprisal))

			switch category.Type {
			case types.CategoryVerticalIgnition,
				types.CategoryFrenzy,
				types.CategoryAggressiveDrive,
				types.CategoryLiquidityShock,
				types.CategoryLoadedImbalance,
				types.CategoryCoiledCompression,
				types.CategoryRiskOnSurge,
				types.CategoryDecoupledAlpha,
				types.CategoryLaminarResonance:
				if boostMetric > 0 {
					categoryFactor += boostMetric
				}

			case types.CategoryExhaustion,
				types.CategoryFadedExhaustion,
				types.CategoryThermalExhaustion,
				types.CategoryMechanicalCollapse,
				types.CategoryActiveReversal,
				types.CategorySpoofTrap,
				types.CategoryToxicBluff:
				penalty := category.Strength * category.Confidence
				exhaustionPenalty = math.Max(exhaustionPenalty, penalty)
			}
		}
	}

	boost = hawkesFactor * pumpFactor * flowFactor * thinFactor * sentimentFactor * categoryFactor
	survival = math.Max(0.0, 1.0-exhaustionPenalty)

	if math.IsNaN(boost) || math.IsInf(boost, 0) || boost <= 0 {
		boost = 1.0
	}

	if math.IsNaN(survival) || math.IsInf(survival, 0) || survival <= 0 {
		survival = 0.0
	}

	return boost, survival
}

func momentumMultiplier(thesis *types.Thesis, symbol string) float64 {
	boost, survival := momentumTerms(thesis, symbol)
	multiplier := boost * survival

	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return 0.0
	}

	return multiplier
}

func getOccupiedSymbols(thesis *types.Thesis, desk *broker.Desk) map[string]struct{} {
	occupied := make(map[string]struct{})

	if desk != nil {
		for position := range desk.Positions() {
			if position.Status != types.CLOSED {
				occupied[position.Holding.Symbol] = struct{}{}
			}
		}
	}

	if thesis != nil && thesis.Lifecycle != nil {
		thesis.Lifecycle.Range(func(key, value any) bool {
			if symbol, ok := key.(string); ok {
				if state, isStr := value.(string); isStr {
					switch state {
					case types.LifecycleEntrySelected,
						types.LifecycleEntrySubmitted,
						types.LifecyclePartiallyEntered,
						types.LifecycleManaging:
						occupied[symbol] = struct{}{}
					}
				}
			}

			return true
		})
	}

	return occupied
}

func isExiting(thesis *types.Thesis, symbol string) bool {
	if thesis == nil || thesis.Lifecycle == nil {
		return false
	}

	if val, ok := thesis.Lifecycle.Load(symbol); ok {
		if state, isStr := val.(string); isStr {
			return state == types.LifecycleExitSelected || state == types.LifecycleExitSubmitted
		}
	}

	return false
}
