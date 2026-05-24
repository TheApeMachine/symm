package causal

import "math"

/*
structuralCoef holds fitted SCM coefficients for price velocity.
*/
type structuralCoef struct {
	intercept float64
	macro     float64
	liquidity float64
	flow      float64
}

const minBackdoorDenominator = 1e-9

func associationEffect(samples []causalSample) float64 {
	flows := make([]float64, len(samples))
	vels := make([]float64, len(samples))

	for index, sample := range samples {
		flows[index] = sample.localFlow
		vels[index] = sample.priceVelocity
	}

	return pearson(flows, vels)
}

/*
backdoorFlowEffect is rung-2 P(velocity | do(flow)) via backdoor adjustment on macro and liquidity.
*/
func backdoorFlowEffect(samples []causalSample) float64 {
	macro := extract(samples, func(sample causalSample) float64 { return sample.macroMomentum })
	liquidity := extract(samples, func(sample causalSample) float64 { return sample.liquidity })
	flows := extract(samples, func(sample causalSample) float64 { return sample.localFlow })
	vels := extract(samples, func(sample causalSample) float64 { return sample.priceVelocity })

	residualVel := residualize(vels, macro, liquidity)
	residualFlow := residualize(flows, macro, liquidity)

	denom := math.Max(dot(residualFlow, residualFlow), minBackdoorDenominator)

	return dot(residualVel, residualFlow) / denom
}

/*
fitStructural estimates the SCM velocity = a + b_m*macro + b_l*liquidity + b_f*flow.
*/
func fitStructural(samples []causalSample) (structuralCoef, bool) {
	macro := extract(samples, func(sample causalSample) float64 { return sample.macroMomentum })
	liquidity := extract(samples, func(sample causalSample) float64 { return sample.liquidity })
	flows := extract(samples, func(sample causalSample) float64 { return sample.localFlow })
	vels := extract(samples, func(sample causalSample) float64 { return sample.priceVelocity })

	coef, ok := ols3(vels, macro, liquidity, flows)

	if !ok {
		return structuralCoef{}, false
	}

	return structuralCoef{
		intercept: coef[0],
		macro:     coef[1],
		liquidity: coef[2],
		flow:      coef[3],
	}, true
}

/*
counterfactualUplift is rung-3 uplift from do(flow = interventionFlow) vs observed flow.
*/
func counterfactualUplift(
	current causalSample,
	coef structuralCoef,
	interventionFlow float64,
) float64 {
	observed := predictVelocity(current, coef, current.localFlow)
	counterfactual := predictVelocity(current, coef, interventionFlow)

	return counterfactual - observed
}

func flowInterventionLevel(samples []causalSample) float64 {
	flows := extract(samples, func(sample causalSample) float64 { return sample.localFlow })

	if len(flows) == 0 {
		return 0
	}

	return percentileSorted(copySorted(flows), 0.75)
}

func predictVelocity(sample causalSample, coef structuralCoef, flow float64) float64 {
	return coef.intercept +
		coef.macro*sample.macroMomentum +
		coef.liquidity*sample.liquidity +
		coef.flow*flow
}

func extract(samples []causalSample, pick func(causalSample) float64) []float64 {
	values := make([]float64, len(samples))

	for index, sample := range samples {
		values[index] = pick(sample)
	}

	return values
}

func residualize(target, macro, liquidity []float64) []float64 {
	coef, ok := ols2(target, macro, liquidity)

	if !ok {
		return target
	}

	residuals := make([]float64, len(target))

	for index := range target {
		fitted := coef[0] + coef[1]*macro[index] + coef[2]*liquidity[index]
		residuals[index] = target[index] - fitted
	}

	return residuals
}

func ols2(target, first, second []float64) ([]float64, bool) {
	if len(target) < minCausalHistory {
		return nil, false
	}

	if len(first) != len(target) || len(second) != len(target) {
		return nil, false
	}

	size := len(target)
	normal := make([][]float64, 3)

	for row := 0; row < 3; row++ {
		normal[row] = make([]float64, 3)
	}

	targetVec := make([]float64, 3)

	for index := 0; index < size; index++ {
		predictors := []float64{1, first[index], second[index]}

		for row := 0; row < 3; row++ {
			targetVec[row] += predictors[row] * target[index]

			for col := 0; col < 3; col++ {
				normal[row][col] += predictors[row] * predictors[col]
			}
		}
	}

	return ridgeSolve(normal, targetVec)
}

func ols3(
	target, first, second, third []float64,
) ([]float64, bool) {
	if len(target) < minCausalHistory {
		return nil, false
	}

	if len(first) != len(target) || len(second) != len(target) || len(third) != len(target) {
		return nil, false
	}

	size := len(target)
	normal := make([][]float64, 4)

	for row := 0; row < 4; row++ {
		normal[row] = make([]float64, 4)
	}

	targetVec := make([]float64, 4)

	for index := 0; index < size; index++ {
		predictors := []float64{1, first[index], second[index], third[index]}

		for row := 0; row < 4; row++ {
			targetVec[row] += predictors[row] * target[index]

			for col := 0; col < 4; col++ {
				normal[row][col] += predictors[row] * predictors[col]
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

	meanLeft := mean(left)
	meanRight := mean(right)
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

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
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
