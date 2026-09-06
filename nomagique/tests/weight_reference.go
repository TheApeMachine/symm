// Test-only copy of the supplied learning/weight.go implementation.
package tests

import (
	"math"

	"fmt"
)

/*
referenceCalibrationLearningPair carries a predicted-vs-actual outcome.
*/
type referenceCalibrationLearningPair struct {
	Predicted float64
	Actual    float64
}

/*
referenceCalibrationTrustWeightOutput reports the current trust state.
*/
type referenceCalibrationTrustWeightOutput struct {
	Value     float64
	Predicted float64
	Actual    float64
	Trust     float64
	Rate      float64
	Count     int
}

/*
referenceCalibrationTrustWeight is a self-adapting rate from prediction error.
*/
type referenceCalibrationTrustWeight struct {
	trust   float64
	prev    float64
	minimum float64
	maximum float64
	rate    float64
	count   int
}

/*
referenceCalibrationNewTrustWeight returns a typed trust-weight learner.
*/
func referenceCalibrationNewTrustWeight() *referenceCalibrationTrustWeight {
	return &referenceCalibrationTrustWeight{}
}

/*
referenceCalibrationWeight returns a typed trust-weight learner.
*/
func referenceCalibrationWeight() *referenceCalibrationTrustWeight {
	return referenceCalibrationNewTrustWeight()
}

/*
Measure updates trust from one prediction outcome.
*/
func (trustWeight *referenceCalibrationTrustWeight) Measure(pair referenceCalibrationLearningPair) (referenceCalibrationTrustWeightOutput, error) {
	predicted, actual, err := referenceCalibrationValidate(pair, "trust-weight")

	if err != nil {
		return referenceCalibrationTrustWeightOutput{}, err
	}

	residual := actual - predicted
	derived := trustWeight.trust

	if trustWeight.count == 0 {
		trustWeight.prev = predicted
		trustWeight.minimum = residual
		trustWeight.maximum = residual
		trustWeight.trust = 1
		trustWeight.count = 1
		derived = trustWeight.trust
	}

	if trustWeight.count > 1 {
		trustWeight.minimum = math.Min(trustWeight.minimum, residual)
		trustWeight.maximum = math.Max(trustWeight.maximum, residual)
		trustWeight.count++
	}

	if trustWeight.count == 1 && residual != trustWeight.minimum {
		trustWeight.minimum = math.Min(trustWeight.minimum, residual)
		trustWeight.maximum = math.Max(trustWeight.maximum, residual)
		trustWeight.count = 2
	}

	span := trustWeight.maximum - trustWeight.minimum

	if trustWeight.count > 1 {
		if span == 0 {
			return referenceCalibrationTrustWeightOutput{}, fmt.Errorf("trust-weight: residual span is zero")
		}

		surprise := math.Abs(residual) / span
		trustWeight.rate = surprise
		targetTrust := math.Max(0, 1-surprise)
		trustWeight.trust += surprise * (targetTrust - trustWeight.trust)
		trustWeight.prev = predicted
		derived = trustWeight.trust
	}

	return referenceCalibrationTrustWeightOutput{
		Value:     derived,
		Predicted: predicted,
		Actual:    actual,
		Trust:     trustWeight.trust,
		Rate:      trustWeight.rate,
		Count:     trustWeight.count,
	}, nil
}

/*
Reset clears learned trust state.
*/
func (trustWeight *referenceCalibrationTrustWeight) Reset() {
	*trustWeight = referenceCalibrationTrustWeight{}
}
