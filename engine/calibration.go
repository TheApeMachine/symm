package engine

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/stats"
)

const (
	defaultCalibrationHalfLife = 5 * time.Minute
	defaultCalibrationTick     = 100 * time.Millisecond
	maxCalibrationSample       = 2.0
)

/*
PredictionCalibrator tracks running actual/predicted return ratios from settled forecasts.
Scale feeds back into signal parameters, not post-hoc confidence output.
*/
type PredictionCalibrator struct {
	scale        adaptive.AlphaEMA
	halfLife     time.Duration
	tickInterval time.Duration
	params       CalibrationParams
}

/*
NewPredictionCalibrator returns a neutral calibrator with injected calibration parameters.
*/
func NewPredictionCalibrator(params CalibrationParams) PredictionCalibrator {
	return PredictionCalibrator{
		halfLife:     defaultCalibrationHalfLife,
		tickInterval: defaultCalibrationTick,
		params:       params,
	}
}

/*
Apply records one settled forecast and updates the running EWMA scale.
Half-life adapts to the signal runway when enough samples exist.
*/
func (calibrator *PredictionCalibrator) Apply(feedback PredictionFeedback) {
	if feedback.Unanchored || feedback.PredictedReturn <= 0 {
		return
	}

	sample, ok := CalibrationStep(feedback.PredictedReturn, feedback.ActualReturn)

	if !ok {
		return
	}

	const maxScale = 2.0

	if sample > maxScale {
		sample = maxScale
	}

	if feedback.Runway > 0 && calibrator.scale.Updates() >= calibrator.params.minCalibrationSamples() {
		calibrator.halfLife = calibrator.params.adaptiveHalfLife(feedback.Runway)
	}

	_ = calibrator.scale.Update(sample, calibrator.ewmaAlpha())
}

/*
Scale returns the current parameter calibration multiplier.
*/
func (calibrator *PredictionCalibrator) Scale() float64 {
	if calibrator.scale.Updates() == 0 {
		return 1
	}

	value := calibrator.scale.Value()

	if value <= 0 {
		return 0
	}

	return value
}

/*
CalibrationStep maps realized move to a calibration sample in [0, maxCalibrationSample].
Wins scale by actual/predicted; losses preserve magnitude via 1+actual/predicted clamped at zero.
*/
func CalibrationStep(predictedReturn, actualReturn float64) (float64, bool) {
	if predictedReturn <= 0 {
		return 0, false
	}

	ratio := actualReturn / predictedReturn

	if actualReturn <= 0 {
		ratio = 1 + ratio
	}

	if ratio < 0 {
		ratio = 0
	}

	if ratio > maxCalibrationSample {
		ratio = maxCalibrationSample
	}

	return ratio, true
}

func (calibrator *PredictionCalibrator) ewmaAlpha() float64 {
	if calibrator.tickInterval <= 0 || calibrator.halfLife <= 0 {
		return 1
	}

	return 1 - math.Exp(-math.Log(2)*calibrator.tickInterval.Seconds()/calibrator.halfLife.Seconds())
}

/*
ConfidenceFence returns the symbol-local upper fence for raw confidence history.
*/
func ConfidenceFence(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	lower, upper := stats.Quartiles(values)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return stats.Max(values)
}

/*
NormalizeConfidence maps raw signal strength into [0, 1] against the symbol-local fence.
Returns 0 until enough history exists to calibrate; never invents certainty on a cold symbol.
*/
func (calibrator *PredictionCalibrator) NormalizeConfidence(rawScore float64, history []float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	if len(history) < calibrator.params.minConfidenceHistory() {
		return 0
	}

	fence := ConfidenceFence(history)

	if fence <= 0 {
		return 0
	}

	if rawScore >= fence {
		return 1
	}

	return rawScore / fence
}
