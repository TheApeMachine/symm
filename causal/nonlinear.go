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
fitNonLinearStructural estimates a non-linear SCM for price velocity.
*/
func fitNonLinearStructural(samples []causalSample) (nonLinearModel, bool) {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return nonLinearModel{}, false
	}

	return fitNonLinearTable(nodeTable, []int{
		macroMomentumNode,
		liquidityNode,
		localFlowNode,
	})
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
	model := nonLinearModel{
		intercept: numericMean(targets),
		stumps:    make([]stumpSplit, 0, nonLinearStumps),
	}

	for stumpIndex := 0; stumpIndex < nonLinearStumps; stumpIndex++ {
		split, gain := bestStump(nodeTable, residuals, features)

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
predictNonLinearVelocity returns the ensemble prediction at one observation.
*/
func predictNonLinearVelocity(sample causalSample, model nonLinearModel, flow float64) float64 {
	prediction, err := model.Predict(sample.nodes[:], localFlowNode, flow)

	if err != nil {
		return 0
	}

	return prediction
}

/*
kernelBackdoorFlowEffect estimates rung-2 uplift with Nadaraya-Watson kernel regression.
*/
func kernelBackdoorFlowEffect(samples []causalSample) float64 {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return 0
	}

	effect, err := nodeTable.KernelBackdoorEffect(
		localFlowNode,
		kernelBandwidth,
		macroMomentumNode,
		liquidityNode,
	)

	if err != nil {
		return 0
	}

	return effect
}

func nonLinearCounterfactualUplift(
	current causalSample,
	model nonLinearModel,
	interventionFlow float64,
) float64 {
	uplift, err := model.CounterfactualUplift(
		current.nodes[:],
		localFlowNode,
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
) (stumpSplit, float64) {
	best := stumpSplit{}
	bestGain := 0.0

	for _, featureIndex := range features {
		for _, row := range nodeTable.rows {
			threshold := featureValue(row, featureIndex)
			leftSum, leftCount, rightSum, rightCount := partitionResiduals(
				nodeTable.rows,
				residuals,
				featureIndex,
				threshold,
			)

			if leftCount == 0 || rightCount == 0 {
				continue
			}

			leftMean := leftSum / float64(leftCount)
			rightMean := rightSum / float64(rightCount)
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

func stumpPrediction(sample causalSample, split stumpSplit) float64 {
	return stumpPredictionRow(sample.nodes[:], split, -1, 0)
}

func stumpPredictionWithFlow(sample causalSample, split stumpSplit, flow float64) float64 {
	return stumpPredictionRow(sample.nodes[:], split, localFlowNode, flow)
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
