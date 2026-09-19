package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewLinearPrediction creates a closure that evaluates a feature row against a fitted affine model.
It closes over the feature indices to extract from the raw row, prepending an implicit core.Unit intercept.
The returned closure takes a tuple of [Coefficients, RawRow] and returns the dot product (prediction).
*/
type LinearPrediction types.Value[[2][]float64, float64]
func NewLinearPrediction(features ...types.Integer) LinearPrediction {
	return func(in [2][]float64) float64 {
		coefficients := in[0]
		rawRow := in[1]

		if len(coefficients) != len(features)+1 {
			return 0.0 // Coefficient length mismatch (needs features + intercept)
		}

		// Prepend intercept
		designRow := make([]float64, 1, len(features)+1)
		designRow[0] = core.Unit

		for _, feat := range features {
			featureIdx := 0
			if feat != nil {
				featureIdx = feat(in)
			}
			if featureIdx < 0 || featureIdx >= len(rawRow) {
				return 0.0 // Feature out of bounds
			}
			designRow = append(designRow, rawRow[featureIdx])
		}

		// Dot product
		sum := 0.0
		for i := 0; i < len(coefficients); i++ {
			sum += coefficients[i] * designRow[i]
		}

		return sum
	}
}
