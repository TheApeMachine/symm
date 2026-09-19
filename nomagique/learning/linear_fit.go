package learning

import (
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewLinearFit creates a stateful closure for fitting an Ordinary Least Squares (OLS) model.
It closes over the tolerance, the feature column indices, and the target column index.
The returned closure takes a slice of rows ([][]float64) and returns the OLS coefficients ([]float64).

The feature matrix is augmented with an implicit intercept (core.Unit) at index 0.
*/
type LinearFit types.Value[[][]float64, []float64]
func NewLinearFit(tolerance float64, features []int, target int) LinearFit {
	ols := algo.NewOLS(tolerance)

	return func(rows [][]float64) []float64 {
		x := make([][]float64, 0, len(rows))
		y := make([][]float64, 0, len(rows))

		for _, row := range rows {
			if target < 0 || target >= len(row) {
				return nil // Target out of bounds
			}

			// Prepend intercept
			designRow := make([]float64, 1, len(features)+1)
			designRow[0] = core.Unit

			// Append selected features
			for _, featureIdx := range features {
				if featureIdx < 0 || featureIdx >= len(row) {
					return nil // Feature out of bounds
				}
				designRow = append(designRow, row[featureIdx])
			}

			x = append(x, designRow)
			y = append(y, []float64{row[target]})
		}

		return ols([2][][]float64{x, y})
	}
}
