package learned

import (
	"errors"
	"math"

	"github.com/theapemachine/symm/numeric/adaptive"
)

/*
Weight is a self-adapting rate. It watches the delta between what it predicted
and what actually happened, and adjusts itself accordingly. When the prediction
error is large, the weight moves faster — it has more to learn. When the error
is small, the weight barely moves — it has already converged. This is second-order
dynamics: a value that learns how fast to change by observing how wrong it was.
Weight expects two signals per call: the predicted value and the actual observed
value. The weight output represents how much trust to place in the current model,
high when predictions match reality, low when they diverge.
*/
type Weight struct {
	value    float64
	errorEMA *adaptive.EMA
	deltaEMA *adaptive.EMA
}

/*
NewWeight creates a new Weight. All internal rates are derived from the prediction
errors the weight observes — no initial configuration is needed.
*/
func NewWeight(rate float64) *Weight {
	return &Weight{
		errorEMA: adaptive.NewEMA(rate),
		deltaEMA: adaptive.NewEMA(rate),
	}
}

/*
Next accepts two values — predicted and actual — and returns the current weight.
The weight adjusts itself based on how large the prediction error is relative to
the error's own history. When the current error is larger than the smoothed historical
error, the weight decreases — the model is getting worse, trust less. When the current
error is smaller, the weight increases — the model is improving, trust more.
*/
func (weight *Weight) Next(
	out float64, values ...float64,
) (float64, error) {
	if weight == nil {
		return 0, errors.New("learned: Weight.Next nil receiver")
	}

	if len(values) < 2 {
		return weight.value, nil
	}

	predicted := values[0]
	actual := values[1]

	// Measure the prediction error.
	predictionError := actual - predicted

	if predictionError < 0 {
		predictionError = -predictionError
	}

	// Smooth the error to get a baseline expectation
	// of how wrong we usually are.
	smoothedError, err := weight.errorEMA.Next(0, predictionError)

	if err != nil {
		return 0, err
	}

	// The adjustment direction: positive when we are
	// doing better than usual, negative when worse.
	var adjustment float64

	if smoothedError > 0 {
		adjustment = (smoothedError - predictionError) / smoothedError
	}

	// Smooth the adjustment so the weight does not
	// jump around on individual observations.
	smoothedAdjustment, err := weight.deltaEMA.Next(0, adjustment)

	if err != nil {
		return 0, err
	}

	weight.value += smoothedAdjustment
	weight.value = math.Max(0, math.Min(1, weight.value))

	return weight.value, nil
}

/*
Reset clears the Weight back to its initial state.
*/
func (weight *Weight) Reset() error {
	if weight == nil {
		return errors.New("learned: Weight.Reset nil receiver")
	}

	weight.value = 0

	if err := weight.errorEMA.Reset(); err != nil {
		return err
	}

	return weight.deltaEMA.Reset()
}
