package market

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

/*
consensusEntry derives a paper entry from the current live signal spectrum when
the configured playbook has no action.
*/
type consensusEntry struct {
	confidenceFloor float64
}

type consensusSummary struct {
	positiveSources   int
	riskSources       int
	classifiedSources int
	positiveScore     float64
	riskScore         float64
}

func newConsensusEntry(measurements []logic.Measurement) *consensusEntry {
	return &consensusEntry{
		confidenceFloor: medianConfidence(measurements),
	}
}

func (consensusEntry *consensusEntry) Action(
	measurements []logic.Measurement,
	holdings *logic.Holdings,
) (*logic.Action, error) {
	if len(measurements) == 0 {
		return nil, nil
	}

	symbol, err := logic.SymbolFromMeasurements(measurements)

	if err != nil {
		return nil, err
	}

	if holdings != nil && holdings.IsHolding(symbol) {
		return nil, nil
	}

	summary := consensusEntry.summarize(measurements)

	if !summary.enters() {
		return nil, nil
	}

	return &logic.Action{
		Type:   logic.ActionMarket,
		Side:   trading.Buy,
		Symbol: symbol,
	}, nil
}

func (consensusEntry *consensusEntry) summarize(
	measurements []logic.Measurement,
) consensusSummary {
	summary := consensusSummary{}

	for _, measurement := range measurements {
		if measurement.Confidence < consensusEntry.confidenceFloor {
			continue
		}

		switch directionForCategory(measurement.Category) {
		case directionPositive:
			summary.positiveSources++
			summary.classifiedSources++
			summary.positiveScore += measurement.Confidence
		case directionRisk:
			summary.riskSources++
			summary.classifiedSources++
			summary.riskScore += measurement.Confidence
		}
	}

	return summary
}

func (summary consensusSummary) enters() bool {
	if summary.classifiedSources == 0 {
		return false
	}

	if summary.positiveSources <= summary.riskSources {
		return false
	}

	return summary.positiveScore > summary.riskScore
}

type categoryDirection uint8

const (
	directionNeutral categoryDirection = iota
	directionPositive
	directionRisk
)

func directionForCategory(category logic.CategoryType) categoryDirection {
	switch category {
	case logic.CategoryLaminar,
		logic.CategoryInertial,
		logic.CategoryFrenzy,
		logic.CategoryOrganic,
		logic.CategoryHiddenAbsorption,
		logic.CategoryAggressiveDrive,
		logic.CategoryLoadedImbalance,
		logic.CategoryInefficientLag,
		logic.CategorySynchronizedDrift,
		logic.CategoryDecoupledMove,
		logic.CategoryVerticalIgnition,
		logic.CategoryCoiledCompression,
		logic.CategoryOrganicTrend,
		logic.CategoryExtremeScarcity,
		logic.CategoryRiskOnSurge,
		logic.CategoryDivergentMove,
		logic.CategoryHardSupport,
		logic.CategorySystemicHerd,
		logic.CategoryDecoupledAlpha,
		logic.CategoryEndogenousAlpha,
		logic.CategoryFragileExpansion:
		return directionPositive
	case logic.CategoryTurbulent,
		logic.CategorySaturation,
		logic.CategoryExhaustion,
		logic.CategoryVolumeStarvation,
		logic.CategorySpoofTrap,
		logic.CategoryBookThinning,
		logic.CategoryAnchorStall,
		logic.CategoryFadedExhaustion,
		logic.CategorySystemicSlump,
		logic.CategoryLiquidityVacuum,
		logic.CategoryToxicBluff,
		logic.CategoryStochasticNoise,
		logic.CategoryDivergentStress,
		logic.CategorySystemicBeta,
		logic.CategoryLiquidityShock,
		logic.CategoryCausalNoise,
		logic.CategoryMechanicalCollapse,
		logic.CategoryThermalExhaustion,
		logic.CategoryActiveReversal:
		return directionRisk
	default:
		return directionNeutral
	}
}

func medianConfidence(measurements []logic.Measurement) float64 {
	var stack [logic.SourceCount]float64
	values := stack[:0]

	if len(measurements) > len(stack) {
		values = make([]float64, 0, len(measurements))
	}

	for _, measurement := range measurements {
		if math.IsNaN(measurement.Confidence) || math.IsInf(measurement.Confidence, 0) {
			continue
		}

		values = append(values, measurement.Confidence)
	}

	if len(values) == 0 {
		return 0
	}

	sort.Float64s(values)
	middle := len(values) / 2

	if len(values)%2 == 1 {
		return values[middle]
	}

	return (values[middle-1] + values[middle]) / 2
}
