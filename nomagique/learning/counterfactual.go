package learning

import (
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewCounterfactual creates a stateful closure that estimates an individual counterfactual Y_{X=x}(u).
It implements abduction (computing the factual residual/noise), intervention, and prediction.

Arguments:
- tolerance: The numerical pivot tolerance for the internal OLS solver.
- features: Slice of integer indices for the predictor columns (which must include the treatment).
- target: The integer index of the outcome column.
- treatment: The integer index of the treatment column.
- level: The numerical scalar value to apply to the treatment column under intervention.

The resulting closure accepts a tuple `[HistoricalRows, FactualRow]`.
It returns `[CounterfactualOutcome, Precision]`, where Precision is an inverse error weight.
*/
type Counterfactual types.Value[[2][][]float64, [2]float64]
func NewCounterfactual(tolerance float64, features []int, target int, treatment int, level float64) Counterfactual {
	fitNode := NewLinearFit(tolerance, features, target)
	predictNode := NewLinearPrediction(features)

	return func(in [2][][]float64) [2]float64 {
		history := in[0]
		factualRow := in[1][0] // Since in[1] is a slice of rows but we only want one actual row

		undefined := [2]float64{math.NaN(), math.NaN()}

		// 1. Abduction / Fitting
		coefficients := fitNode(history)
		if coefficients == nil {
			return undefined // Model undefined
		}

		if target < 0 || target >= len(factualRow) {
			return undefined
		}

		factualOutcome := factualRow[target]

		// 2. Compute Factual Residual (Noise 'u')
		factualPrediction := predictNode([2][]float64{coefficients, factualRow})
		noise := factualOutcome - factualPrediction

		// 3. Intervention (do X=x)
		intervened := make([]float64, len(factualRow))
		copy(intervened, factualRow)

		if treatment < 0 || treatment >= len(intervened) {
			return undefined
		}
		intervened[treatment] = level

		// 4. Counterfactual Prediction (using identical noise 'u')
		counterfactualPrediction := predictNode([2][]float64{coefficients, intervened})
		counterfactualOutcome := counterfactualPrediction + noise

		// Precision is a measure of audit weight: 1 / (1 + |noise|)
		precision := core.Unit / (core.Unit + math.Abs(noise))

		return [2]float64{counterfactualOutcome, precision}
	}
}
