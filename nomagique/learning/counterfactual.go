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
func NewCounterfactual(tolerance types.Float, target, treatment types.Integer, level types.Float, features ...types.Integer) Counterfactual {
	fitNode := NewLinearFit(tolerance, target, features...)
	predictNode := NewLinearPrediction(features...)

	return func(in [2][][]float64) [2]float64 {
		history := in[0]
		factualRow := in[1][0] // Since in[1] is a slice of rows but we only want one actual row

		undefined := [2]float64{math.NaN(), math.NaN()}

		// 1. Abduction / Fitting
		coefficients := fitNode(history)
		if coefficients == nil {
			return undefined // Model undefined
		}

		t := 0
		if target != nil {
			t = target(in)
		}
		treat := 0
		if treatment != nil {
			treat = treatment(in)
		}
		lev := 0.0
		if level != nil {
			lev = level(in)
		}

		if t < 0 || t >= len(factualRow) {
			return undefined
		}

		factualOutcome := factualRow[t]

		// 2. Compute Factual Residual (Noise 'u')
		factualPrediction := predictNode([2][]float64{coefficients, factualRow})
		noise := factualOutcome - factualPrediction

		// 3. Intervention (do X=x)
		intervened := make([]float64, len(factualRow))
		copy(intervened, factualRow)

		if treat < 0 || treat >= len(intervened) {
			return undefined
		}
		intervened[treat] = lev

		// 4. Counterfactual Prediction (using identical noise 'u')
		counterfactualPrediction := predictNode([2][]float64{coefficients, intervened})
		counterfactualOutcome := counterfactualPrediction + noise

		// 5. Precision
		precision := core.Unit / (1.0 + math.Abs(noise))

		return [2]float64{counterfactualOutcome, precision}
	}
}
