package causal

import "errors"

const (
	nonLinearStumps = 8
	kernelBandwidth = 0.35
	minKernelWeight = 1e-9
)

type stumpSplit struct {
	featureIndex int
	threshold    float64
	leftMean     float64
	rightMean    float64
}

/*
nonLinearModel is a gradient-boosted stump ensemble for velocity prediction.
*/
type nonLinearModel struct {
	intercept float64
	stumps    []stumpSplit
}

/*
fitNonLinearStructuralFor estimates a non-linear SCM for price velocity for a regime.
*/
func fitNonLinearStructuralFor(samples []causalSample, roles causalRoles) (nonLinearModel, bool) {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return nonLinearModel{}, false
	}

	return fitNonLinearTable(nodeTable, roles.predictors())
}

func fitNonLinearTable(
	nodeTable dagNodeTable,
	features []int,
) (nonLinearModel, bool) {
	targets, err := nodeTable.column(nodeTable.target)

	if err != nil {
		return nonLinearModel{}, false
	}

	residuals := append([]float64(nil), targets...)
	thresholds := featureThresholds(nodeTable, features)
	model := nonLinearModel{
		intercept: numericMean(targets),
		stumps:    make([]stumpSplit, 0, nonLinearStumps),
	}

	for stumpIndex := 0; stumpIndex < nonLinearStumps; stumpIndex++ {
		split, gain := bestStump(nodeTable, residuals, features, thresholds)

		if gain <= 0 {
			break
		}

		model.stumps = append(model.stumps, split)

		for index, row := range nodeTable.rows {
			residuals[index] -= stumpPredictionRow(row, split, -1, 0)
		}
	}

	return model, len(model.stumps) > 0
}

func numericMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
}

/*
kernelBackdoorEffectFor estimates rung-2 uplift with Nadaraya-Watson kernel regression for a regime.
*/
func kernelBackdoorEffectFor(samples []causalSample, roles causalRoles) float64 {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return 0
	}

	return kernelBackdoorEffectFromTable(nodeTable, roles)
}

func kernelBackdoorEffectFromTable(nodeTable dagNodeTable, roles causalRoles) float64 {
	effect, err := nodeTable.KernelBackdoorEffect(
		roles.treatment,
		kernelBandwidth,
		roles.controls...,
	)

	if err != nil {
		return 0
	}

	return effect
}

func nonLinearCounterfactualUpliftFor(
	current causalSample,
	model nonLinearModel,
	interventionFlow float64,
	roles causalRoles,
) float64 {
	uplift, err := model.CounterfactualUplift(
		current.nodes[:],
		roles.treatment,
		interventionFlow,
	)

	if err != nil {
		return 0
	}

	return uplift
}

func bestStump(
	nodeTable dagNodeTable,
	residuals []float64,
	features []int,
	thresholds map[int][]float64,
) (stumpSplit, float64) {
	best := stumpSplit{}
	bestGain := 0.0

	for _, featureIndex := range features {
		for _, threshold := range thresholds[featureIndex] {
			leftSum, leftCount, rightSum, rightCount := partitionResiduals(
				nodeTable.rows,
				residuals,
				featureIndex,
				threshold,
			)

			if leftCount == 0 || rightCount == 0 {
				continue
			}

			leftMean := leftSum / leftCount
			rightMean := rightSum / rightCount
			gain := splitGain(
				residuals,
				leftMean,
				rightMean,
				nodeTable.rows,
				featureIndex,
				threshold,
			)

			if gain <= bestGain {
				continue
			}

			bestGain = gain
			best = stumpSplit{
				featureIndex: featureIndex,
				threshold:    threshold,
				leftMean:     leftMean,
				rightMean:    rightMean,
			}
		}
	}

	return best, bestGain
}

func featureThresholds(nodeTable dagNodeTable, features []int) map[int][]float64 {
	thresholds := make(map[int][]float64, len(features))

	for _, featureIndex := range features {
		seen := make(map[float64]struct{}, len(nodeTable.rows))

		for _, row := range nodeTable.rows {
			value := featureValue(row, featureIndex)
			seen[value] = struct{}{}
		}

		values := make([]float64, 0, len(seen))

		for value := range seen {
			values = append(values, value)
		}

		thresholds[featureIndex] = values
	}

	return thresholds
}

func partitionResiduals(
	rows [][]float64,
	residuals []float64,
	featureIndex int,
	threshold float64,
) (leftSum, leftCount, rightSum, rightCount float64) {
	for index, row := range rows {
		if featureValue(row, featureIndex) <= threshold {
			leftSum += residuals[index]
			leftCount++
			continue
		}

		rightSum += residuals[index]
		rightCount++
	}

	return leftSum, leftCount, rightSum, rightCount
}

func splitGain(
	residuals []float64,
	leftMean, rightMean float64,
	rows [][]float64,
	featureIndex int,
	threshold float64,
) float64 {
	before := 0.0
	after := 0.0

	for index, row := range rows {
		residual := residuals[index]
		before += residual * residual
		prediction := rightMean

		if featureValue(row, featureIndex) <= threshold {
			prediction = leftMean
		}

		delta := residual - prediction
		after += delta * delta
	}

	return before - after
}

func stumpPredictionRow(
	row []float64,
	split stumpSplit,
	overrideNode int,
	overrideValue float64,
) float64 {
	value := featureValueWithOverride(row, split.featureIndex, overrideNode, overrideValue)

	if value <= split.threshold {
		return split.leftMean
	}

	return split.rightMean
}

func featureValue(row []float64, featureIndex int) float64 {
	return featureValueWithOverride(row, featureIndex, -1, 0)
}

func featureValueWithOverride(
	row []float64,
	featureIndex int,
	overrideNode int,
	overrideValue float64,
) float64 {
	if featureIndex == overrideNode {
		return overrideValue
	}

	return row[featureIndex]
}

func (model nonLinearModel) Predict(
	row []float64,
	overrideNode int,
	overrideValue float64,
) (float64, error) {
	prediction := model.intercept

	for _, split := range model.stumps {
		if split.featureIndex < 0 || split.featureIndex >= len(row) {
			return 0, errors.New("causal: stump feature outside row")
		}

		prediction += stumpPredictionRow(row, split, overrideNode, overrideValue)
	}

	return prediction, nil
}

func (model nonLinearModel) CounterfactualUplift(
	row []float64,
	treatment int,
	intervention float64,
) (float64, error) {
	observed, err := model.Predict(row, -1, 0)

	if err != nil {
		return 0, err
	}

	counterfactual, err := model.Predict(row, treatment, intervention)

	if err != nil {
		return 0, err
	}

	return counterfactual - observed, nil
}
