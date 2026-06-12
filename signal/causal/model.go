package causal

import (
	"math"

	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/statistic"
)

const minBackdoorDenominator = 1e-9
const minPredictorScale = 1e-12

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
	scaledPredictors, predictorMeans, predictorScales := standardizePredictors(predictors)

	for index := 0; index < size; index++ {
		rowValues[0] = 1

		for predictorIndex := range scaledPredictors {
			rowValues[predictorIndex+1] = scaledPredictors[predictorIndex][index]
		}

		for row := 0; row < width; row++ {
			targetVec[row] += rowValues[row] * target[index]

			for col := 0; col < width; col++ {
				normal[row][col] += rowValues[row] * rowValues[col]
			}
		}
	}

	solutionScaled, err := NewRidgeSolver().Solve(normal, targetVec)

	if err != nil {
		return nil, false
	}

	return unscaleOLSCoefficients(solutionScaled, predictorMeans, predictorScales), true
}

func standardizePredictors(predictors [][]float64) (
	scaled [][]float64,
	means []float64,
	scales []float64,
) {
	scaled = make([][]float64, len(predictors))
	means = make([]float64, len(predictors))
	scales = make([]float64, len(predictors))

	for predictorIndex, predictor := range predictors {
		mean := float64(statistic.NewMean(nil).Observe(
			nomagique.Numbers(predictor...)...,
		))
		scale := populationStdDev(predictor, mean)

		if scale < minPredictorScale {
			scale = minPredictorScale
		}

		means[predictorIndex] = mean
		scales[predictorIndex] = scale
		scaled[predictorIndex] = make([]float64, len(predictor))

		for index := range predictor {
			scaled[predictorIndex][index] = (predictor[index] - mean) / scale
		}
	}

	return scaled, means, scales
}

func unscaleOLSCoefficients(
	solutionScaled []float64,
	means []float64,
	scales []float64,
) []float64 {
	solution := make([]float64, len(solutionScaled))
	copy(solution, solutionScaled)

	for predictorIndex := range means {
		solution[0] -= solutionScaled[predictorIndex+1] * means[predictorIndex] / scales[predictorIndex]
		solution[predictorIndex+1] = solutionScaled[predictorIndex+1] / scales[predictorIndex]
	}

	return solution
}

func populationStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sumSquares := 0.0

	for _, value := range values {
		delta := value - mean
		sumSquares += delta * delta
	}

	return math.Sqrt(sumSquares / float64(len(values)))
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
