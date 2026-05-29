package causal

import (
	"math"

	"github.com/theapemachine/symm/numeric"
)

/*
structuralCoef holds fitted SCM coefficients for price velocity.
*/
type structuralCoef struct {
	model dagLinearModel
}

const minBackdoorDenominator = 1e-9

/*
associationEffect is rung-1 P(velocity | treatment): observational correlation in the normal
regime. associationEffectFor reads the same correlation for an arbitrary regime's treatment.
*/
func associationEffect(samples []causalSample) float64 {
	return associationEffectFor(samples, normalRoles())
}

func associationEffectFor(samples []causalSample, roles causalRoles) float64 {
	nodeTable, err := causalTableWithMin(samples, 1)

	if err != nil {
		return 0
	}

	association, err := nodeTable.Association(roles.treatment)

	if err != nil {
		return 0
	}

	return association
}

/*
backdoorFlowEffect is rung-2 P(velocity | do(treatment)) via backdoor adjustment. The normal
regime controls macro and liquidity; backdoorEffectFor adjusts on whatever controls the supplied
regime declares.
*/
func backdoorFlowEffect(samples []causalSample) float64 {
	return backdoorEffectFor(samples, normalRoles())
}

func backdoorEffectFor(samples []causalSample, roles causalRoles) float64 {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return 0
	}

	effect, err := nodeTable.BackdoorEffect(roles.treatment, roles.controls...)

	if err != nil {
		return 0
	}

	return effect
}

/*
fitStructural estimates the SCM velocity = a + Σ b*predictors for the normal regime
(macro, liquidity, flow). fitStructuralFor fits the predictor set of any regime.
*/
func fitStructural(samples []causalSample) (structuralCoef, bool) {
	return fitStructuralFor(samples, normalRoles())
}

func fitStructuralFor(samples []causalSample, roles causalRoles) (structuralCoef, bool) {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return structuralCoef{}, false
	}

	model, err := nodeTable.LinearModel(roles.predictors()...)

	if err != nil {
		return structuralCoef{}, false
	}

	return structuralCoef{model: model}, true
}

/*
counterfactualUplift is rung-3 uplift from do(treatment = intervention) vs observed treatment.
*/
func counterfactualUplift(
	current causalSample,
	coef structuralCoef,
	interventionFlow float64,
) float64 {
	return counterfactualUpliftFor(current, coef, interventionFlow, normalRoles())
}

func counterfactualUpliftFor(
	current causalSample,
	coef structuralCoef,
	interventionFlow float64,
	roles causalRoles,
) float64 {
	uplift, err := coef.model.CounterfactualUplift(
		current.nodes[:],
		roles.treatment,
		interventionFlow,
	)

	if err != nil {
		return 0
	}

	return uplift
}

func flowInterventionLevel(samples []causalSample) float64 {
	return flowInterventionLevelFor(samples, normalRoles())
}

func flowInterventionLevelFor(samples []causalSample, roles causalRoles) float64 {
	nodeTable, err := causalTableWithMin(samples, 1)

	if err != nil {
		return 0
	}

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

func extract(samples []causalSample, node int) []float64 {
	values := make([]float64, len(samples))

	for index := range samples {
		values[index] = samples[index].value(node)
	}

	return values
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

func ols3(target, first, second, third []float64) ([]float64, bool) {
	return ols(target, first, second, third)
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

	return ridgeSolve(normal, targetVec)
}

func gaussianSolve(matrix [][]float64, vector []float64) ([]float64, bool) {
	size := len(vector)
	augmented := make([][]float64, size)

	for row := 0; row < size; row++ {
		augmented[row] = make([]float64, size+1)
		copy(augmented[row], matrix[row])
		augmented[row][size] = vector[row]
	}

	for pivot := 0; pivot < size; pivot++ {
		maxRow := pivot
		maxMag := math.Abs(augmented[pivot][pivot])

		for row := pivot + 1; row < size; row++ {
			magnitude := math.Abs(augmented[row][pivot])

			if magnitude > maxMag {
				maxMag = magnitude
				maxRow = row
			}
		}

		if maxMag <= solverPivotFloor {
			return nil, false
		}

		if maxRow != pivot {
			augmented[pivot], augmented[maxRow] = augmented[maxRow], augmented[pivot]
		}

		pivotValue := augmented[pivot][pivot]

		for col := pivot; col <= size; col++ {
			augmented[pivot][col] /= pivotValue
		}

		for row := 0; row < size; row++ {
			if row == pivot {
				continue
			}

			factor := augmented[row][pivot]

			for col := pivot; col <= size; col++ {
				augmented[row][col] -= factor * augmented[pivot][col]
			}
		}
	}

	solution := make([]float64, size)

	for row := 0; row < size; row++ {
		solution[row] = augmented[row][size]
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
