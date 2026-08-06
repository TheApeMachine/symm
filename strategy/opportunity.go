package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

func highVelocityOpportunity(thesis *types.Thesis, symbol string) bool {
	if thesis == nil {
		return false
	}

	stored, found := thesis.Categories.Load(symbol)

	if !found {
		return false
	}

	categories, ok := stored.([]types.Category)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"opportunity: category map holds a value that is not a category set",
			nil,
		))

		return false
	}

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
