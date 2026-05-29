package causal

import "math"

const (
	minConditionRatio = 100.0
	ridgeScaleFactor  = 0.01
	solverPivotFloor  = 1e-12
)

/*
ridgeSolve solves (X'X + λI)β = X'y with intercept left unpenalized.
λ scales with estimated condition number of the normal matrix.
*/
func ridgeSolve(normal [][]float64, vector []float64) ([]float64, bool) {
	size := len(vector)
	regularized := make([][]float64, size)

	for row := 0; row < size; row++ {
		regularized[row] = make([]float64, size)
		copy(regularized[row], normal[row])
	}

	lambda := ridgeLambda(normal)

	for row := 1; row < size; row++ {
		regularized[row][row] += lambda
	}

	return gaussianSolve(regularized, vector)
}

func ridgeLambda(normal [][]float64) float64 {
	trace := 0.0
	size := float64(len(normal))

	for row := 0; row < len(normal); row++ {
		trace += normal[row][row]
	}

	if trace <= 0 || size <= 0 {
		return 0
	}

	base := trace / size
	condition := conditionEstimate(normal)
	extra := 0.0

	if condition > minConditionRatio || math.IsInf(condition, 0) {
		extra = base * ridgeScaleFactor
	}

	return base*1e-8 + extra
}

func conditionEstimate(normal [][]float64) float64 {
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
