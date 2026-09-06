// Test-only copy of the supplied learning/sample_ratio.go implementation.
package tests

import (
	"math"

	"fmt"
)

/*
referenceCalibrationSampleRatioOutput reports calibration ratio state.
*/
type referenceCalibrationSampleRatioOutput struct {
	Value     float64
	Predicted float64
	Actual    float64
	PeakRatio float64
	Count     int
}

/*
referenceCalibrationCalibrator tracks calibration sample ratio from predicted-vs-actual pairs.
*/
type referenceCalibrationCalibrator struct {
	prev      float64
	minimum   float64
	maximum   float64
	peakRatio float64
	count     int
}

/*
referenceCalibrationSampleRatio returns a typed calibration learner.
*/
func referenceCalibrationSampleRatio() *referenceCalibrationCalibrator {
	return &referenceCalibrationCalibrator{}
}

/*
Measure updates calibration ratio from one prediction outcome.
*/
func (calibrator *referenceCalibrationCalibrator) Measure(pair referenceCalibrationLearningPair) (referenceCalibrationSampleRatioOutput, error) {
	predicted, actual, err := referenceCalibrationValidate(pair, "sample-ratio")

	if err != nil {
		return referenceCalibrationSampleRatioOutput{}, err
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
			return referenceCalibrationSampleRatioOutput{}, fmt.Errorf("sample-ratio: loss ratio is negative")
		}
	}

	ceiling := 1.0

	if span > 0 {
		ceiling = 1 + 1/span
	}

	if span == 0 && math.Abs(calibrator.prev) > 0 {
		ceiling = 1 + 1/math.Abs(calibrator.prev)
	}

	if ratio > ceiling {
		ratio = ceiling
	}

	if ratio > calibrator.peakRatio {
		calibrator.peakRatio = ratio
	}

	calibrator.prev = predicted

	return referenceCalibrationSampleRatioOutput{
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
func (calibrator *referenceCalibrationCalibrator) Reset() {
	*calibrator = referenceCalibrationCalibrator{}
}
