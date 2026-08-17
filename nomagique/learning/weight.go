package learning

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
LearningPair carries a predicted-vs-actual outcome.
*/
type LearningPair struct {
	Predicted float64
	Actual    float64
}

/*
TrustWeightOutput reports the current trust state.
*/
type TrustWeightOutput struct {
	Value     float64
	Predicted float64
	Actual    float64
	Trust     float64
	Rate      float64
	Count     int
}

/*
TrustWeight is a self-adapting rate from prediction error.
*/
type TrustWeight struct {
	trust   float64
	prev    float64
	minimum float64
	maximum float64
	rate    float64
	count   int
}

/*
NewTrustWeight returns a typed trust-weight learner.
*/
func NewTrustWeight() *TrustWeight {
	return &TrustWeight{}
}

/*
Weight returns a typed trust-weight learner.
*/
func Weight() *TrustWeight {
	return NewTrustWeight()
}

/*
Measure updates trust from one prediction outcome.
*/
func (trustWeight *TrustWeight) Measure(pair LearningPair) (TrustWeightOutput, error) {
	predicted, actual, err := validatePair(pair, "trust-weight")

	if err != nil {
		return TrustWeightOutput{}, err
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
			return TrustWeightOutput{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"trust-weight: residual span is zero",
				nil,
			))
		}

		surprise := absExact(residual) / span
		trustWeight.rate = surprise
		targetTrust := math.Max(0, 1-surprise)
		trustWeight.trust += surprise * (targetTrust - trustWeight.trust)
		trustWeight.prev = predicted
		derived = trustWeight.trust
	}

	return TrustWeightOutput{
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
func (trustWeight *TrustWeight) Reset() {
	*trustWeight = TrustWeight{}
}
