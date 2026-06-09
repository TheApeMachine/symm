package causal

import (
	"math"

	"github.com/theapemachine/symm/numeric"
)

const minBackdoorDenominator = 1e-9

func associationEffectFromTable(nodeTable dagNodeTable, roles causalRoles) float64 {
	association, err := nodeTable.Association(roles.treatment)

	if err != nil {
		return 0
	}

	return association
}

func flowInterventionLevelFromTable(nodeTable dagNodeTable, roles causalRoles) float64 {
	value, err := nodeTable.Percentile(roles.treatment, 0.75)

	if err != nil {
		return 0
	}

	return value
}

func causalTable(samples []causalSample) (dagNodeTable, error) {
	return causalTableWithMin(samples, minCausalHistory)
}

func causalTableWithMin(samples []causalSample, minRows int) (dagNodeTable, error) {
	rows := make([][]float64, len(samples))

	for index := range samples {
		rows[index] = samples[index].nodes[:]
	}

	return newDAGNodeTable(rows, priceVelocityNode, minRows)
}

func residualize(target []float64, controls ...[]float64) ([]float64, bool) {
	if len(controls) == 0 {
		return append([]float64(nil), target...), true
	}

	coef, ok := ols(target, controls...)

	if !ok {
		return nil, false
	}

	residuals := make([]float64, len(target))

	for index := range target {
		fitted := coef[0]

		for controlIndex, control := range controls {
			fitted += coef[controlIndex+1] * control[index]
		}

		residuals[index] = target[index] - fitted
	}

	return residuals, true
}

func ols(target []float64, predictors ...[]float64) ([]float64, bool) {
	if len(target) < minCausalHistory {
		return nil, false
	}

	for _, predictor := range predictors {
		if len(predictor) != len(target) {
			return nil, false
		}
	}

	size := len(target)
	width := len(predictors) + 1
	normal := make([][]float64, width)

	for row := range width {
		normal[row] = make([]float64, width)
	}

	targetVec := make([]float64, width)
	rowValues := make([]float64, width)

	for index := 0; index < size; index++ {
		rowValues[0] = 1

		for predictorIndex, predictor := range predictors {
			rowValues[predictorIndex+1] = predictor[index]
		}

		for row := 0; row < width; row++ {
			targetVec[row] += rowValues[row] * target[index]

			for col := 0; col < width; col++ {
				normal[row][col] += rowValues[row] * rowValues[col]
			}
		}
	}

	solution, err := NewRidgeSolver().Solve(normal, targetVec)

	if err != nil {
		return nil, false
	}

	return solution, true
}

func pearson(left, right []float64) float64 {
	if len(left) != len(right) || len(left) == 0 {
		return 0
	}

	meanLeft := numeric.Mean(left)
	meanRight := numeric.Mean(right)
	numerator := 0.0
	varLeft := 0.0
	varRight := 0.0

	for index := range left {
		deltaLeft := left[index] - meanLeft
		deltaRight := right[index] - meanRight
		numerator += deltaLeft * deltaRight
		varLeft += deltaLeft * deltaLeft
		varRight += deltaRight * deltaRight
	}

	denom := math.Sqrt(varLeft * varRight)

	if denom <= 0 {
		return 0
	}

	return numerator / denom
}

func dot(left, right []float64) float64 {
	if len(left) != len(right) {
		return 0
	}

	sum := 0.0

	for index := range left {
		sum += left[index] * right[index]
	}

	return sum
}
