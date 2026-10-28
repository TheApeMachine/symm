package causal

import (
	"errors"
	"sort"

	"gonum.org/v1/gonum/stat"
)

const nonLinearStumps = 8

type stumpSplit struct {
	featureIndex int
	threshold    float64
	leftMean     float64
	rightMean    float64
}

type nonLinearModel struct {
	intercept   float64
	stumps      []stumpSplit
	hasLinear   bool
	linearModel linearModel
}

func fitNonLinearTable(nodeTable nodeTable, features []int) (nonLinearModel, bool) {
	if len(features) == 0 {
		return nonLinearModel{}, false
	}

	if err := nodeTable.validateNodes(features...); err != nil {
		return nonLinearModel{}, false
	}

	targets, err := nodeTable.column(nodeTable.target)

	if err != nil {
		return nonLinearModel{}, false
	}

	// 1. Fit robust linear model using Ridge regression
	linearModel, err := nodeTable.fitLinearModel(features...)

	if err != nil {
		return nonLinearModel{}, false
	}

	// 2. Compute residuals from the linear fit
	residuals := make([]float64, len(nodeTable.rows))

	for index, row := range nodeTable.rows {
		pred, err := linearModel.predict(row, -1, 0)

		if err != nil {
			return nonLinearModel{}, false
		}

		residuals[index] = targets[index] - pred
	}

	// Compute standard deviation of residuals for regularization/anomaly thresholding
	residualStd := stat.StdDev(residuals, nil)

	thresholds := featureThresholds(nodeTable, features)
	model := nonLinearModel{
		intercept:   0.0, // Managed by linearModel
		stumps:      make([]stumpSplit, 0, nonLinearStumps),
		hasLinear:   true,
		linearModel: linearModel,
	}

	for stumpIndex := 0; stumpIndex < nonLinearStumps; stumpIndex++ {
		split, gain := bestStump(nodeTable, residuals, features, thresholds)

		// Regularization: splitGain is a total SSE reduction, so compare it to
		// the current residual variance on the same total-sample scale.
		residualStd = stat.StdDev(residuals, nil)
		minGain := 0.01 * (residualStd * residualStd) * float64(len(residuals))

		if gain <= minGain {
			break
		}

		model.stumps = append(model.stumps, split)

		for index, row := range nodeTable.rows {
			residuals[index] -= stumpPredictionRow(row, split, -1, 0)
		}
	}

	return model, true
}

func (model nonLinearModel) predict(
	row []float64, overrideNode int, overrideValue float64,
) (float64, error) {
	var prediction float64
	var err error

	if model.hasLinear {
		prediction, err = model.linearModel.predict(row, overrideNode, overrideValue)

		if err != nil {
			return 0, err
		}
	} else {
		prediction = model.intercept
	}

	for _, split := range model.stumps {
		if split.featureIndex < 0 || split.featureIndex >= len(row) {
			return 0, errors.New("causal: stump feature outside row")
		}

		prediction += stumpPredictionRow(row, split, overrideNode, overrideValue)
	}

	return prediction, nil
}

func (model nonLinearModel) counterfactualUplift(
	row []float64, treatment int, intervention float64,
) (float64, error) {
	observed, err := model.predict(row, -1, 0)

	if err != nil {
		return 0, err
	}

	counterfactual, err := model.predict(row, treatment, intervention)

	if err != nil {
		return 0, err
	}

	return counterfactual - observed, nil
}

func bestStump(
	nodeTable nodeTable,
	residuals []float64,
	features []int,
	thresholds map[int][]float64,
) (stumpSplit, float64) {
	best := stumpSplit{}
	bestGain := 0.0

	for _, featureIndex := range features {
		for _, threshold := range thresholds[featureIndex] {
			leftSum, leftCount, rightSum, rightCount := partitionResiduals(
				nodeTable.rows, residuals, featureIndex, threshold,
			)

			if leftCount == 0 || rightCount == 0 {
				continue
			}

			leftMean := leftSum / leftCount
			rightMean := rightSum / rightCount
			gain := splitGain(
				residuals, leftMean, rightMean, nodeTable.rows, featureIndex, threshold,
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

func featureThresholds(nodeTable nodeTable, features []int) map[int][]float64 {
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

		sort.Float64s(values)
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

/*
featureValueWithOverride reads one feature out of a row, substituting the
override when the requested index is the intervened node.

A row too short to carry the requested feature reads as zero rather than
panicking. Rows reach the forest from callers that assemble them per observation,
so a partial observation produces a short row, and a library that indexes it
unchecked turns a gap in the caller's data into a crash in the caller's process.
Zero is the neutral reading for a feature that was not observed: it places the
row on the split's left branch without asserting anything the data did not say.
*/
func featureValueWithOverride(
	row []float64,
	featureIndex int,
	overrideNode int,
	overrideValue float64,
) float64 {
	if featureIndex == overrideNode {
		return overrideValue
	}

	if featureIndex < 0 || featureIndex >= len(row) {
		return 0
	}

	return row[featureIndex]
}
