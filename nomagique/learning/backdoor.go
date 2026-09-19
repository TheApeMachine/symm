package learning

import (
	"math"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewBackdoor creates a stateful closure that estimates the interventional expectation E[Y | do(X=x)].
It encapsulates the regression (LinearFit), intervention, and expectation (Mean) in one pure closure.
The design enforces valid feature inputs via closures without relying on structs like Query.

Arguments:
- tolerance: The numerical pivot tolerance for the internal OLS solver.
- features: Slice of integer indices for the predictor columns (which must include the treatment).
- target: The integer index of the outcome column.
- treatment: The integer index of the treatment column.
- level: The numerical scalar value to apply to the treatment column under intervention.

The resulting closure accepts an observation matrix ([][]float64) representing the historical data,
and returns the expected value of the target under the intervention.
*/
type Backdoor types.Value[[][]float64, float64]
func NewBackdoor(tolerance types.Float, target, treatment types.Integer, level types.Float, features ...types.Integer) Backdoor {
	// Initialize the structural elements
	fitNode := NewLinearFit(tolerance, target, features...)
	predictNode := NewLinearPrediction(features...)

	return func(rows [][]float64) float64 {
		// 1. Abduction / Fitting
		coefficients := fitNode(rows)
		if coefficients == nil {
			return math.NaN() // Fit is rank-deficient or undefined
		}

		treat := 0
		if treatment != nil {
			treat = treatment(rows)
		}
		lev := 0.0
		if level != nil {
			lev = level(rows)
		}

		// 2. Intervention and Prediction
		meanNode := statistic.NewMean()
		var expectation float64

		for _, row := range rows {
			// Clone the row and apply the intervention: do(treatment = level)
			intervened := make([]float64, len(row))
			copy(intervened, row)

			if treat < 0 || treat >= len(intervened) {
				return math.NaN()
			}
			intervened[treat] = lev

			// Predict the counterfactual outcome for this intervened row
			prediction := predictNode([2][]float64{coefficients, intervened})

			// Accumulate expectation
			expectation = meanNode(prediction)
		}

		return expectation
	}
}
