package causal

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

const (
	minConditionRatio = 100.0
	ridgeScaleFactor  = 0.01
	solverPivotFloor  = 1e-12
)

/*
RidgeSolver solves regularized normal equations with Gonum LAPACK backends.
*/
type RidgeSolver struct{}

func NewRidgeSolver() *RidgeSolver {
	return &RidgeSolver{}
}

func (solver *RidgeSolver) Solve(normal [][]float64, vector []float64) ([]float64, error) {
	size := len(vector)

	if size == 0 || len(normal) != size {
		return nil, fmt.Errorf("causal: ridge system size mismatch")
	}

	for row := 0; row < size; row++ {
		if len(normal[row]) != size {
			return nil, fmt.Errorf("causal: ridge normal matrix is not square")
		}
	}

	lambda := solver.ridgeLambda(normal)
	symmetric := mat.NewSymDense(size, nil)

	for row := 0; row < size; row++ {
		for col := row; col < size; col++ {
			value := normal[row][col]

			if row == col && row > 0 {
				value += lambda
			}

			symmetric.SetSym(row, col, value)
		}
	}

	rightHand := mat.NewVecDense(size, vector)

	var cholesky mat.Cholesky

	if cholesky.Factorize(symmetric) {
		var solution mat.VecDense

		if solveErr := cholesky.SolveVecTo(&solution, rightHand); solveErr != nil {
			return nil, fmt.Errorf("causal: cholesky solve: %w", solveErr)
		}

		out := make([]float64, size)

		for index := range out {
			out[index] = solution.AtVec(index)
		}

		return out, nil
	}

	var qr mat.QR

	if qr.Factorize(mat.NewDense(size, size, flattenSquare(normal))) {
		var solution mat.VecDense

		if solveErr := qr.SolveVecTo(&solution, false, rightHand); solveErr != nil {
			return nil, fmt.Errorf("causal: qr solve: %w", solveErr)
		}

		out := make([]float64, size)

		for index := range out {
			out[index] = solution.AtVec(index)
		}

		return out, nil
	}

	return nil, fmt.Errorf("causal: ridge factorization failed")
}

func flattenSquare(matrix [][]float64) []float64 {
	size := len(matrix)
	flat := make([]float64, size*size)

	for row := 0; row < size; row++ {
		copy(flat[row*size:(row+1)*size], matrix[row])
	}

	return flat
}

func (solver *RidgeSolver) ridgeLambda(normal [][]float64) float64 {
	trace := 0.0
	size := float64(len(normal))

	for row := 0; row < len(normal); row++ {
		trace += normal[row][row]
	}

	if trace <= 0 || size <= 0 {
		return 0
	}

	base := trace / size
	condition := solver.conditionEstimate(normal)
	extra := 0.0

	if condition > minConditionRatio || math.IsInf(condition, 0) {
		extra = base * ridgeScaleFactor
	}

	return base*1e-8 + extra
}

func (solver *RidgeSolver) conditionEstimate(normal [][]float64) float64 {
	size := len(normal)

	if size == 0 {
		return 0
	}

	diagonals := make([]float64, size)

	for row := 0; row < size; row++ {
		if len(normal[row]) != size {
			return math.Inf(1)
		}

		diagonals[row] = normal[row][row]

		if diagonals[row] <= 0 {
			return math.Inf(1)
		}
	}

	maxEigenBound := 0.0
	minEigenBound := math.Inf(1)

	for row := 0; row < size; row++ {
		radius := 0.0

		for col := 0; col < size; col++ {
			if col == row {
				continue
			}

			normalizer := math.Sqrt(diagonals[row] * diagonals[col])

			if normalizer <= 0 {
				return math.Inf(1)
			}

			radius += math.Abs(normal[row][col]) / normalizer
		}

		upper := 1 + radius
		lower := 1 - radius

		if upper > maxEigenBound {
			maxEigenBound = upper
		}

		if lower < minEigenBound {
			minEigenBound = lower
		}
	}

	if minEigenBound <= solverPivotFloor {
		return math.Inf(1)
	}

	return maxEigenBound / minEigenBound
}
