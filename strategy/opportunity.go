package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

func highVelocityOpportunity(
	thesis *types.Thesis,
	symbol string,
) (bool, bool) {
	if thesis == nil {
		return false, false
	}

	if adverseOpportunity(thesis, symbol) {
		return false, true
	}

	stored, found := thesis.Categories.Load(symbol)

	if !found {
		return false, false
	}

	categories, ok := stored.([]types.Category)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"opportunity: category map holds a value that is not a category set",
			nil,
		))

		return false, false
	}

	highVelocity := false

	for _, category := range categories {
		switch category.Type {
		case types.CategoryExhaustion,
			types.CategoryFadedExhaustion,
			types.CategoryThermalExhaustion,
			types.CategoryMechanicalCollapse,
			types.CategoryActiveReversal,
			types.CategoryHiddenAbsorption,
			types.CategorySpoofTrap,
			types.CategoryToxicBluff,
			types.CategoryBookThinning:
			return false, true
		case types.CategoryVerticalIgnition,
			types.CategoryFrenzy,
			types.CategoryAggressiveDrive,
			types.CategoryLiquidityShock,
			types.CategoryLoadedImbalance:
			highVelocity = true
		}
	}

	return highVelocity, false
}

/*
adverseOpportunity reads classifications the flow and book models have already
made before the category layer has enough independent evidence to name them.
Only relative dominance is used: absorption must exceed drive, while a thin or
spoof book must outrank every competing book state.
*/
func adverseOpportunity(thesis *types.Thesis, symbol string) bool {
	flow, flowReady := latestMeasurement(thesis, symbol, types.SourceCVD)

	if flowReady {
		absorption, absorptionReady := rawMetric(
			flow, types.MetricAbsorption, types.SideNone,
		)
		drive, driveReady := rawMetric(flow, types.MetricDrive, types.SideNone)

		if absorptionReady && driveReady && absorption > drive {
			return true
		}
	}

	book, bookReady := latestMeasurement(thesis, symbol, types.SourceDepthFlow)

	if !bookReady {
		return false
	}

	loaded, loadedReady := rawMetric(book, types.MetricLoadedScore, types.SideNone)
	spoof, spoofReady := rawMetric(book, types.MetricSpoofScore, types.SideNone)
	thin, thinReady := rawMetric(book, types.MetricThinScore, types.SideNone)
	neutral, neutralReady := rawMetric(book, types.MetricNeutralScore, types.SideNone)

	if !loadedReady || !spoofReady || !thinReady || !neutralReady {
		return false
	}

	competing := math.Max(loaded, neutral)

	return thin > math.Max(spoof, competing) ||
		spoof > math.Max(thin, competing)
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
