package algo

import (
	"fmt"
	"iter"
	"math"

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
A singular system is Defined=false with an empty Solution, not a guess.
*/
type Solution struct {
	Solution [][]float64
	Rank     int
	Defined  bool
}

/*
GaussJordan owns partial-pivot elimination. Tolerance is the absolute pivot
floor of this solver, a representation bound, not a market threshold.
*/
type GaussJordan struct {
	core.Base[System, Solution]
	tolerance float64
}

func NewGaussJordan(tolerance float64) *GaussJordan {
	return &GaussJordan{tolerance: tolerance}
}

func (op *GaussJordan) Next(
	in iter.Seq[core.Primitive[System, System]],
) iter.Seq[core.Primitive[Solution, Solution]] {
	return func(yield func(core.Primitive[Solution, Solution]) bool) {
		for arriving := range in {
			solution, err := op.Solve(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(solution)) {
				return
			}
		}
	}
}

/*
Solve reduces [Left|Right] in isolation. Inputs are not mutated.
*/
func (op *GaussJordan) Solve(system System) (Solution, error) {
	rows := len(system.Left)

	if rows == 0 {
		return Solution{}, fmt.Errorf("%w: gauss-jordan requires a square left matrix", core.ErrShape)
	}

	if op.tolerance < 0 {
		return Solution{}, fmt.Errorf("%w: gauss-jordan tolerance is negative", core.ErrDomain)
	}

	if len(system.Right) != rows {
		return Solution{}, fmt.Errorf("%w: gauss-jordan right-hand rows differ", core.ErrShape)
	}

	width := len(system.Left[0])
	rightColumns := len(system.Right[0])

	if width != rows {
		return Solution{}, fmt.Errorf("%w: gauss-jordan left matrix is not square", core.ErrShape)
	}

	if rightColumns == 0 {
		return Solution{}, fmt.Errorf("%w: gauss-jordan right-hand side is empty", core.ErrShape)
	}

	for _, row := range system.Left {
		if len(row) != width {
			return Solution{}, fmt.Errorf("%w: gauss-jordan left matrix is ragged", core.ErrShape)
		}
	}

	for _, row := range system.Right {
		if len(row) != rightColumns {
			return Solution{}, fmt.Errorf("%w: gauss-jordan right-hand side is ragged", core.ErrShape)
		}
	}

	columns := rows + rightColumns
	augmented := make([][]float64, rows)
	storage := make([]float64, rows*columns)

	for row := range rows {
		augmented[row] = storage[row*columns : (row+1)*columns]
		copy(augmented[row], system.Left[row])
		copy(augmented[row][rows:], system.Right[row])
	}

	rank := 0

	for pivot := 0; pivot < rows; pivot++ {
		winner := pivot
		magnitude := math.Abs(augmented[pivot][pivot])

		for row := pivot + 1; row < rows; row++ {
			candidate := math.Abs(augmented[row][pivot])

			if candidate > magnitude {
				winner = row
				magnitude = candidate
			}
		}

		if !(magnitude > op.tolerance) {
			return Solution{Rank: rank, Defined: false}, nil
		}

		if winner != pivot {
			augmented[pivot], augmented[winner] = augmented[winner], augmented[pivot]
		}

		scale := 1 / augmented[pivot][pivot]

		for column := pivot; column < columns; column++ {
			augmented[pivot][column] *= scale
		}

		for row := 0; row < rows; row++ {
			if row == pivot {
				continue
			}

			factor := augmented[row][pivot]

			if factor == 0 {
				continue
			}

			for column := pivot; column < columns; column++ {
				augmented[row][column] -= factor * augmented[pivot][column]
			}
		}

		rank++
	}

	solution := make([][]float64, rows)
	values := make([]float64, rows*rightColumns)

	for row := range rows {
		solution[row] = values[row*rightColumns : (row+1)*rightColumns]
		copy(solution[row], augmented[row][rows:])
	}

	return Solution{Solution: solution, Rank: rank, Defined: true}, nil
}
