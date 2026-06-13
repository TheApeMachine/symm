package logic

import "math"

const defaultSafetyMarginBps = 5.0

const minEdgeConfidenceGate = 0.55

/*
EdgeObservation captures forecast residual semantics separate from transition novelty.
*/
type EdgeObservation struct {
	PredictedMoveBps float64
	RealizedMoveBps  float64
	ResidualBps      float64
	EdgeSurprise     float64
}

/*
DynamicCostBps estimates round-trip execution cost from spread, fees, and slippage.
*/
func DynamicCostBps(
	spreadBps float64,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) float64 {
	if safetyMarginBps <= 0 {
		safetyMarginBps = defaultSafetyMarginBps
	}

	return spreadBps/2 + feeBps + projectedSlippageBps + safetyMarginBps
}

/*
ExpectedEdgeBps returns cost-adjusted expected move in basis points.
*/
func ExpectedEdgeBps(
	predictedMoveBps float64,
	spreadBps float64,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) float64 {
	costBps := DynamicCostBps(spreadBps, feeBps, projectedSlippageBps, safetyMarginBps)

	return predictedMoveBps - costBps
}

/*
SpreadBpsFromMeasurement converts a touch spread ratio into basis points.
*/
func SpreadBpsFromMeasurement(measurement Measurement) float64 {
	if measurement.Price <= 0 || measurement.Spread <= 0 {
		return 0
	}

	return measurement.Spread / measurement.Price * 10000
}

/*
MeetsExpectedEdgeGate rejects entries unless calibrated expected move exceeds costs.
Strength is never treated as a basis-point forecast.
*/
func MeetsExpectedEdgeGate(
	measurements []Measurement,
	executionCost ExecutionCost,
) bool {
	for _, measurement := range measurements {
		if !edgeProviderEligible(measurement) {
			continue
		}

		if measurement.ExpectedMoveBps <= 0 {
			continue
		}

		if measurement.EdgeConfidence <= 0 {
			continue
		}

		edgeBps := measurement.ExpectedMoveBps - executionCost.TotalBps

		if edgeBps > 0 && measurement.EdgeConfidence >= minEdgeConfidenceGate {
			return true
		}
	}

	return false
}

func edgeProviderEligible(measurement Measurement) bool {
	if measurement.Source == SourceNone {
		return false
	}

	if measurement.DecisionGrade == DecisionGradeEdgeProvider {
		return measurement.ExpectedMoveBps > 0
	}

	if measurement.DecisionGrade != DecisionGradeExecutable {
		return false
	}

	if measurement.Category == CategoryTypeNone {
		return false
	}

	return isEntryOrientedSource(measurement.Source)
}

func clampUnit(value, lower, upper float64) float64 {
	if value < lower {
		return lower
	}

	if value > upper {
		return upper
	}

	return value
}

func logit(probability float64) float64 {
	return math.Log(probability / (1 - probability))
}
