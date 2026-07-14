package logic

import (
	"math"

	"github.com/theapemachine/symm/types"
)

func (composer *CategoryComposer) toxicityState(
	symbol string,
	measurements []*types.Measurement,
	graph *types.Graph,
) (types.Category, bool) {
	bidCancelled, hasBid := latestSideMeasurement(
		symbol, measurements,
		types.SubjectLevel3Touch, types.MetricCancelledQuantity, types.SideBuy,
	)
	askCancelled, hasAsk := latestSideMeasurement(
		symbol, measurements,
		types.SubjectLevel3Touch, types.MetricCancelledQuantity, types.SideSell,
	)

	if !hasBid && !hasAsk {
		return types.Category{}, false
	}

	bidPressure := cancelPressure(bidCancelled, hasBid)
	askPressure := cancelPressure(askCancelled, hasAsk)
	totalPressure := bidPressure + askPressure

	if totalPressure <= 0 {
		return types.Category{}, false
	}

	anchor := bidCancelled

	if askPressure > bidPressure && hasAsk {
		anchor = askCancelled
	}

	categoryType := types.CategoryHardSupport
	dominantPressure := math.Max(bidPressure, askPressure)
	recessivePressure := math.Min(bidPressure, askPressure)

	if dominantPressure > recessivePressure {
		categoryType = types.CategoryToxicBluff
	}

	anchorKey := types.MeasurementKey(anchor)
	supporting, opposing := graphEvidence(graph, anchorKey)
	missing := missingSubjects(symbol, measurements, []types.SubjectType{
		types.SubjectBookImbalance,
		types.SubjectPumpIgnition,
	})

	strength := dominantPressure / totalPressure
	confidence := evidenceConfidence(
		anchor.Maturity,
		len(supporting),
		len(opposing),
		len(missing),
	)

	return types.Category{
		Symbol:     symbol,
		Type:       categoryType,
		Strength:   strength,
		Confidence: confidence,
		Surprisal:  1 - confidence,
		Maturity:   anchor.Maturity,
		Supporting: supporting,
		Opposing:   opposing,
		Missing:    missing,
	}, true
}

func cancelPressure(
	cancelled *types.Measurement,
	hasCancel bool,
) float64 {
	if !hasCancel || cancelled == nil {
		return 0
	}

	if cancelled.Normalized != nil {
		return math.Abs(*cancelled.Normalized) * cancelled.Maturity
	}

	return cancelled.Raw * cancelled.Maturity
}
