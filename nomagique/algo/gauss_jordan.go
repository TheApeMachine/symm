package algo

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
System is a square left-hand matrix and its right-hand sides.
*/
type System struct {
	Left  [][]float64
	Right [][]float64
}

/*
Solution is the reduced right-hand side, or an explicit rank-deficient state.
A singular system is Defined=false with an empty Solution.
*/
type Solution struct {
	Solution [][]float64
	Rank     int
	Defined  bool
}

/*
GaussJordan owns partial-pivot elimination. Tolerance is the absolute pivot
floor of this solver.
*/
type GaussJordan struct {
	*core.PrimitiveError

	tolerance float64
	out       Solution
}

func NewGaussJordan(tolerance float64) *GaussJordan {
	return &GaussJordan{PrimitiveError: core.NewPrimitiveError(), tolerance: tolerance}
}

func (gaussJordan *GaussJordan) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			system := (*System)(arriving)
			sol, err := gaussJordan.solve(*system)

			if err != nil {
				gaussJordan.Error(err)
				return
			}

			gaussJordan.out = sol

			if !yield(unsafe.Pointer(&gaussJordan.out)) {
				return
			}
		}
	}
}

/*
solve performs row reduction with partial pivoting.
*/
func (gaussJordan *GaussJordan) solve(system System) (Solution, error) {
	rows := len(system.Left)

	if rows == 0 {
		return Solution{Solution: [][]float64{}, Defined: false}, nil
	}

	for _, row := range system.Left {
		if len(row) != rows {
			return Solution{}, fmt.Errorf("%w: Gauss-Jordan left-hand side must be square", core.ErrShape)
		}
	}

	if len(system.Right) != rows {
		return Solution{}, fmt.Errorf("%w: Gauss-Jordan right-hand row count differs from left", core.ErrShape)
	}

	rightCols := 0

	if rows > 0 {
		rightCols = len(system.Right[0])
	}

	for _, row := range system.Right {
		if len(row) != rightCols {
			return Solution{}, fmt.Errorf("%w: Gauss-Jordan right-hand side is ragged", core.ErrShape)
		}
	}

	// Clone matrices
	a := make([][]float64, rows)
	b := make([][]float64, rows)

	for i := range rows {
		a[i] = make([]float64, rows)
		copy(a[i], system.Left[i])
		b[i] = make([]float64, rightCols)
		copy(b[i], system.Right[i])
	}

	rank := 0

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

		if maxVal <= gaussJordan.tolerance {
			return Solution{Solution: [][]float64{}, Rank: rank, Defined: false}, nil
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

		for r := 0; r < rows; r++ {
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

		rank++
	}

	return Solution{Solution: b, Rank: rank, Defined: true}, nil
}
