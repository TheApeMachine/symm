package algo

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewOLS creates a closure for Ordinary Least Squares (OLS) regression.
It takes a tuple of [DesignMatrix, TargetVector] and returns the Coefficients vector.
If the system is singular or rank-deficient, it returns nil.
The design matrix X must have dimensions (Observations x Parameters).
The target vector Y must have dimensions (Observations).
*/
type OLS types.Value[[2][][]float64, []float64]
func NewOLS(tolerance float64) OLS {
	solver := NewGaussJordan(tolerance)

	return func(in [2][][]float64) []float64 {
		x := in[0]
		// To match GaussJordan signature which expects [][], we assume in[1] is a column vector
		// of shape (Observations x 1).
		y := in[1]

		observations := len(x)
		if observations == 0 {
			return nil
		}
		parameters := len(x[0])

		if len(y) != observations {
			return nil
		}

		// Compute X^T * X (Left matrix)
		xtx := make([][]float64, parameters)
		for i := 0; i < parameters; i++ {
			xtx[i] = make([]float64, parameters)
			for j := 0; j < parameters; j++ {
				sum := 0.0
				for k := 0; k < observations; k++ {
					sum += x[k][i] * x[k][j]
				}
				xtx[i][j] = sum
			}
		}

		// Compute X^T * Y (Right matrix)
		xty := make([][]float64, parameters)
		for i := 0; i < parameters; i++ {
			xty[i] = make([]float64, 1)
			sum := 0.0
			for k := 0; k < observations; k++ {
				sum += x[k][i] * y[k][0]
			}
			xty[i][0] = sum
		}

		// Solve (X^T * X) * beta = X^T * Y
		solution := solver([2][][]float64{xtx, xty})

		if solution == nil {
			return nil
		}

		// Flatten the result back to a 1D coefficient vector
		beta := make([]float64, parameters)
		for i := 0; i < parameters; i++ {
			beta[i] = solution[i][0]
		}

		return beta
	}
}
