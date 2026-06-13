package logic

import "math"

const defaultSafetyMarginBps = 5.0

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
ExecutionCostFromMeasurements derives spread-based cost from the measurement spectrum.
*/
func ExecutionCostFromMeasurements(
	measurements []Measurement,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) float64 {
	spreadBps := 0.0

	for _, measurement := range measurements {
		candidateSpread := SpreadBpsFromMeasurement(measurement)

		if candidateSpread > spreadBps {
			spreadBps = candidateSpread
		}
	}

	return DynamicCostBps(spreadBps, feeBps, projectedSlippageBps, safetyMarginBps)
}

/*
MeetsExpectedEdgeGate rejects entries whose predicted move does not exceed costs.
When no prediction measurement is present, a strength-based proxy must clear costs.
*/
func MeetsExpectedEdgeGate(
	measurements []Measurement,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) bool {
	costBps := ExecutionCostFromMeasurements(
		measurements,
		feeBps,
		projectedSlippageBps,
		safetyMarginBps,
	)

	for _, measurement := range measurements {
		if measurement.Source != SourcePrediction {
			continue
		}

		predictedMoveBps := measurement.ExpectedMoveBps

		if predictedMoveBps <= 0 {
			predictedMoveBps = math.Abs(measurement.Strength) * 10000
		}

		return ExpectedEdgeBps(
			predictedMoveBps,
			SpreadBpsFromMeasurement(measurement),
			feeBps,
			projectedSlippageBps,
			safetyMarginBps,
		) > 0
	}

	for _, measurement := range measurements {
		if !isEntryOrientedSource(measurement.Source) {
			continue
		}

		proxyMoveBps := measurement.ExpectedMoveBps

		if proxyMoveBps <= 0 {
			proxyMoveBps = measurement.Strength * 10000
		}

		if proxyMoveBps > costBps {
			return true
		}
	}

	return false
}
