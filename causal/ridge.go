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

	if condition > minConditionRatio {
		extra = base * (condition/minConditionRatio - 1) * ridgeScaleFactor
	}

	return base*1e-8 + extra
}

func conditionEstimate(normal [][]float64) float64 {
	if len(normal) == 0 {
		return 0
	}

	maxRowSum := 0.0
	minRowSum := math.Inf(1)

	for row := 0; row < len(normal); row++ {
		rowSum := 0.0

		for col := 0; col < len(normal[row]); col++ {
			rowSum += math.Abs(normal[row][col])
		}

		if rowSum > maxRowSum {
			maxRowSum = rowSum
		}

		if rowSum < minRowSum {
			minRowSum = rowSum
		}
	}

	if minRowSum <= solverPivotFloor {
		return math.Inf(1)
	}

	return maxRowSum / minRowSum
}
