package causal

import "math"

const (
	nonLinearStumps   = 8
	kernelBandwidth   = 0.35
	minKernelWeight   = 1e-9
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
	if len(samples) < minCausalHistory {
		return nonLinearModel{}, false
	}

	targets := extract(samples, func(sample causalSample) float64 { return sample.priceVelocity })
	residuals := append([]float64(nil), targets...)
	model := nonLinearModel{
		intercept: mean(targets),
		stumps:    make([]stumpSplit, 0, nonLinearStumps),
	}

	for stumpIndex := 0; stumpIndex < nonLinearStumps; stumpIndex++ {
		split, gain := bestStump(samples, residuals)

		if gain <= 0 {
			break
		}

		model.stumps = append(model.stumps, split)

		for index, sample := range samples {
			residuals[index] -= stumpPrediction(sample, split)
		}
	}

	return model, len(model.stumps) > 0
}

/*
predictNonLinearVelocity returns the ensemble prediction at one observation.
*/
func predictNonLinearVelocity(
	sample causalSample,
	model nonLinearModel,
	flow float64,
) float64 {
	prediction := model.intercept

	for _, split := range model.stumps {
		prediction += stumpPredictionWithFlow(sample, split, flow)
	}

	return prediction
}

/*
kernelBackdoorFlowEffect estimates rung-2 uplift with Nadaraya-Watson kernel regression.
*/
func kernelBackdoorFlowEffect(samples []causalSample) float64 {
	if len(samples) < minCausalHistory {
		return 0
	}

	current := samples[len(samples)-1]
	numerator := 0.0
	denominator := 0.0

	for _, sample := range samples {
		distance := featureDistance(current, sample)
		weight := math.Exp(-distance * distance / (2 * kernelBandwidth * kernelBandwidth))

		if weight < minKernelWeight {
			continue
		}

		numerator += weight * sample.priceVelocity * sample.localFlow
		denominator += weight * sample.localFlow * sample.localFlow
	}

	if denominator <= 0 {
		return 0
	}

	return numerator / denominator
}

func bestStump(samples []causalSample, residuals []float64) (stumpSplit, float64) {
	best := stumpSplit{}
	bestGain := 0.0

	for featureIndex := 0; featureIndex < 3; featureIndex++ {
		for _, sample := range samples {
			threshold := featureValue(sample, featureIndex)

			leftSum, leftCount, rightSum, rightCount := partitionResiduals(
				samples, residuals, featureIndex, threshold,
			)

			if leftCount == 0 || rightCount == 0 {
				continue
			}

			leftMean := leftSum / float64(leftCount)
			rightMean := rightSum / float64(rightCount)
			gain := splitGain(residuals, leftMean, rightMean, samples, featureIndex, threshold)

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
	samples []causalSample,
	residuals []float64,
	featureIndex int,
	threshold float64,
) (leftSum, leftCount, rightSum, rightCount float64) {
	for index, sample := range samples {
		if featureValue(sample, featureIndex) <= threshold {
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
	samples []causalSample,
	featureIndex int,
	threshold float64,
) float64 {
	gain := 0.0

	for index, sample := range samples {
		residual := residuals[index]
		prediction := rightMean

		if featureValue(sample, featureIndex) <= threshold {
			prediction = leftMean
		}

		gain += (residual - prediction) * (residual - prediction)
	}

	return -gain
}

func stumpPrediction(sample causalSample, split stumpSplit) float64 {
	return stumpPredictionWithFlow(sample, split, sample.localFlow)
}

func stumpPredictionWithFlow(
	sample causalSample,
	split stumpSplit,
	flow float64,
) float64 {
	value := featureValueWithFlow(sample, split.featureIndex, flow)

	if value <= split.threshold {
		return split.leftMean
	}

	return split.rightMean
}

func featureValue(sample causalSample, featureIndex int) float64 {
	return featureValueWithFlow(sample, featureIndex, sample.localFlow)
}

func featureValueWithFlow(sample causalSample, featureIndex int, flow float64) float64 {
	switch featureIndex {
	case 0:
		return sample.macroMomentum
	case 1:
		return sample.liquidity
	default:
		return flow
	}
}

func featureDistance(left, right causalSample) float64 {
	macroDelta := left.macroMomentum - right.macroMomentum
	liquidityDelta := left.liquidity - right.liquidity
	flowDelta := left.localFlow - right.localFlow

	return math.Sqrt(macroDelta*macroDelta + liquidityDelta*liquidityDelta + flowDelta*flowDelta)
}

func nonLinearCounterfactualUplift(
	current causalSample,
	model nonLinearModel,
	interventionFlow float64,
) float64 {
	observed := predictNonLinearVelocity(current, model, current.localFlow)
	counterfactual := predictNonLinearVelocity(current, model, interventionFlow)

	return counterfactual - observed
}
