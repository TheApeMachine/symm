package logic

/*
ExecutionCost captures round-trip execution assumptions used consistently by
entry gates, candidate scoring, and sizing.
*/
type ExecutionCost struct {
	SpreadBps            float64
	FeeBps               float64
	ProjectedSlippageBps float64
	SafetyMarginBps      float64
	TotalBps             float64
}

/*
ExecutionCostFromMarket derives execution cost from live measurements and fee policy.
*/
func ExecutionCostFromMarket(
	measurements []Measurement,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) ExecutionCost {
	spreadBps := 0.0

	for _, measurement := range measurements {
		candidateSpread := SpreadBpsFromMeasurement(measurement)

		if candidateSpread > spreadBps {
			spreadBps = candidateSpread
		}
	}

	totalBps := DynamicCostBps(
		spreadBps,
		feeBps,
		projectedSlippageBps,
		safetyMarginBps,
	)

	return ExecutionCost{
		SpreadBps:            spreadBps,
		FeeBps:               feeBps,
		ProjectedSlippageBps: projectedSlippageBps,
		SafetyMarginBps:      safetyMarginBps,
		TotalBps:             totalBps,
	}
}

/*
ExecutionCostFromMeasurements preserves the legacy total-bps helper.
*/
func ExecutionCostFromMeasurements(
	measurements []Measurement,
	feeBps float64,
	projectedSlippageBps float64,
	safetyMarginBps float64,
) float64 {
	return ExecutionCostFromMarket(
		measurements,
		feeBps,
		projectedSlippageBps,
		safetyMarginBps,
	).TotalBps
}
