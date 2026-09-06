// Test-only copy of the supplied joint estimator.
package tests

import (
	"math"
)

/*
referenceJointEstimator maintains independent online Welford estimators for a fixed-size
vector of log-space observations. It provides per-channel baselines, residuals,
z-scores, and a joint Mahalanobis-style SNR across all channels.

Baseline/Residual/ZScore are causal: they report the prior mean BEFORE the
current observation was incorporated (the same contract as CausalResidual).

This is a pure v2 equation: zero Frame, zero Symbol, zero MustIntern. All state
is value-embedded in the struct.
*/
type referenceJointEstimator struct {
	engines    [3]referenceMoments
	lastValues [3]float64
	priorMeans [3]float64
	priorDisps [3]float64
	count      float64
	horizon    int64
}

func (joint *referenceJointEstimator) HasMean() bool {
	return joint.count > 0
}

func (joint *referenceJointEstimator) Step(values [3]float64, sec, nsec float64) {
	for index := range joint.engines {
		joint.priorMeans[index] = joint.engines[index].Mean()
		joint.priorDisps[index] = joint.engines[index].Dispersion()
		joint.engines[index].Update(values[index])
		joint.lastValues[index] = values[index]
	}
	joint.count++
	joint.horizon = int64(sec)*1_000_000_000 + int64(nsec)
}

func (joint *referenceJointEstimator) Baseline(index int) float64 {
	return math.Exp(joint.priorMeans[index])
}

func (joint *referenceJointEstimator) Ratio(index int) float64 {
	return math.Exp(joint.lastValues[index] - joint.priorMeans[index])
}

func (joint *referenceJointEstimator) Residual(index int) float64 {
	return joint.lastValues[index] - joint.priorMeans[index]
}

func (joint *referenceJointEstimator) Noise(index int) (float64, bool) {
	disp := joint.priorDisps[index]

	if disp <= 0 || joint.count < 3 {
		return 0, false
	}

	return disp, true
}

func (joint *referenceJointEstimator) ZScore(index int) (float64, bool) {
	disp := joint.priorDisps[index]

	if disp <= 0 || joint.count < 3 {
		return 0, false
	}

	return (joint.lastValues[index] - joint.priorMeans[index]) / disp, true
}

func (joint *referenceJointEstimator) SNR() (float64, bool) {
	if joint.count < 3 {
		return 0, false
	}

	sum := 0.0
	defined := 0

	for index := range joint.engines {
		disp := joint.priorDisps[index]

		if disp <= 0 {
			continue
		}

		residual := joint.lastValues[index] - joint.priorMeans[index]
		sum += (residual * residual) / (disp * disp)
		defined++
	}

	if defined == 0 {
		return 0, false
	}

	return sum / float64(defined), true
}

func (joint *referenceJointEstimator) NEff() float64 {
	return joint.count
}

func (joint *referenceJointEstimator) Horizon() int64 {
	return joint.horizon
}
