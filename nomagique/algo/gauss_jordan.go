package algo

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewGaussJordan creates a closure that performs row reduction with partial pivoting.
It takes a tuple of [LeftMatrix, RightMatrix] and returns the reduced RightMatrix.
If the system is singular or rank-deficient based on the given tolerance, it returns nil.
*/
type GaussJordan types.Value[[2][][]float64, [][]float64]
func NewGaussJordan(tolerance float64) GaussJordan {
	return func(in [2][][]float64) [][]float64 {
		left := in[0]
		right := in[1]
		rows := len(left)

		if rows == 0 {
			return nil
		}

		for _, row := range left {
			if len(row) != rows {
				return nil // Must be square
			}
		}

		if len(right) != rows {
			return nil // Row count must match
		}

		rightCols := 0
		if rows > 0 {
			rightCols = len(right[0])
		}

		for _, row := range right {
			if len(row) != rightCols {
				return nil // Right hand side is ragged
			}
		}

		// Clone matrices
		a := make([][]float64, rows)
		b := make([][]float64, rows)
		
		for i := range rows {
			a[i] = make([]float64, rows)
			copy(a[i], left[i])
			b[i] = make([]float64, rightCols)
			copy(b[i], right[i])
		}

		for col := 0; col < rows; col++ {
			pivotRow := col
			maxVal := math.Abs(a[col][col])

			for r := col + 1; r < rows; r++ {
				val := math.Abs(a[r][col])
				if val > maxVal {
					maxVal = val
					pivotRow = r
				}
			}

			if maxVal <= tolerance {
				return nil // Rank deficient
			}

			if pivotRow != col {
				a[col], a[pivotRow] = a[pivotRow], a[col]
				b[col], b[pivotRow] = b[pivotRow], b[col]
			}

			pivot := a[col][col]
			for c := col; c < rows; c++ {
				a[col][c] /= pivot
			}
			for c := 0; c < rightCols; c++ {
				b[col][c] /= pivot
			}

			for r := range rows {
				if r != col {
					factor := a[r][col]
					for c := col; c < rows; c++ {
						a[r][c] -= factor * a[col][c]
					}
					for c := 0; c < rightCols; c++ {
						b[r][c] -= factor * b[col][c]
					}
				}
			}
		}

		return b
	}
}
