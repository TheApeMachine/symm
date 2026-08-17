package learning

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
SampleRatioOutput reports calibration ratio state.
*/
type SampleRatioOutput struct {
	Value     float64
	Predicted float64
	Actual    float64
	PeakRatio float64
	Count     int
}

/*
Calibrator tracks calibration sample ratio from predicted-vs-actual pairs.
*/
type Calibrator struct {
	prev      float64
	minimum   float64
	maximum   float64
	peakRatio float64
	count     int
}

/*
SampleRatio returns a typed calibration learner.
*/
func SampleRatio() *Calibrator {
	return &Calibrator{}
}

/*
Measure updates calibration ratio from one prediction outcome.
*/
func (calibrator *Calibrator) Measure(pair LearningPair) (SampleRatioOutput, error) {
	predicted, actual, err := validatePair(pair, "sample-ratio")

	if err != nil {
		return SampleRatioOutput{}, err
	}

	residual := actual - predicted

	if calibrator.count == 0 {
		calibrator.minimum = residual
		calibrator.maximum = residual
		calibrator.prev = predicted
		calibrator.count = 1
	}

	if calibrator.count > 1 {
		calibrator.minimum = math.Min(calibrator.minimum, residual)
		calibrator.maximum = math.Max(calibrator.maximum, residual)
		calibrator.count++
	}

	if calibrator.count == 1 && residual != calibrator.minimum {
		calibrator.minimum = math.Min(calibrator.minimum, residual)
		calibrator.maximum = math.Max(calibrator.maximum, residual)
		calibrator.count = 2
	}

	span := calibrator.maximum - calibrator.minimum
	ratio := actual / predicted

	if actual < predicted {
		ratio = 1 + actual/predicted

		if ratio < 0 {
			return SampleRatioOutput{}, errnie.Error(errnie.Err(
				errnie.Validation,
				"sample-ratio: loss ratio is negative",
				nil,
			))
		}
	}

	ceiling := 1.0

	if span > 0 {
		ceiling = 1 + 1/span
	}

	if span == 0 && absExact(calibrator.prev) > 0 {
		ceiling = 1 + 1/absExact(calibrator.prev)
	}

	if ratio > ceiling {
		ratio = ceiling
	}

	if ratio > calibrator.peakRatio {
		calibrator.peakRatio = ratio
	}

	calibrator.prev = predicted

	return SampleRatioOutput{
		Value:     ratio,
		Predicted: predicted,
		Actual:    actual,
		PeakRatio: calibrator.peakRatio,
		Count:     calibrator.count,
	}, nil
}

/*
Reset clears calibration state.
*/
func (calibrator *Calibrator) Reset() {
	*calibrator = Calibrator{}
}
