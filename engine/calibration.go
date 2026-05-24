package engine

import (
	"math"
	"time"

	"github.com/theapemachine/symm/stats"
)

const (
	defaultCalibrationHalfLife = 5 * time.Minute
	defaultCalibrationTick     = 100 * time.Millisecond
)

/*
PredictionCalibrator tracks running actual/predicted return ratios from settled forecasts.
Scale feeds back into signal parameters, not post-hoc confidence output.
*/
type PredictionCalibrator struct {
	feedbackCount int
	scale         float64
	halfLife      time.Duration
	tickInterval  time.Duration
}

/*
NewPredictionCalibrator returns a neutral calibrator with EWMA defaults.
*/
func NewPredictionCalibrator() PredictionCalibrator {
	return PredictionCalibrator{
		scale:        1,
		halfLife:     defaultCalibrationHalfLife,
		tickInterval: defaultCalibrationTick,
	}
}

/*
Apply records one settled forecast and updates the running EWMA scale.
*/
func (calibrator *PredictionCalibrator) Apply(feedback PredictionFeedback) {
	if feedback.Unanchored || feedback.PredictedReturn <= 0 {
		return
	}

	sample := CalibrationStep(feedback.PredictedReturn, feedback.ActualReturn)

	if sample <= 0 {
		return
	}

	calibrator.feedbackCount++

	if calibrator.feedbackCount == 1 {
		calibrator.scale = sample
		return
	}

	alpha := calibrator.ewmaAlpha()
	calibrator.scale += alpha * (sample - calibrator.scale)
}

/*
Scale returns the current parameter calibration multiplier.
*/
func (calibrator *PredictionCalibrator) Scale() float64 {
	if calibrator.feedbackCount == 0 {
		return 1
	}

	if calibrator.scale <= 0 {
		return 0
	}

	return calibrator.scale
}

/*
CalibrationStep is actualReturn/predictedReturn for one signed settled forecast.
*/
func CalibrationStep(predictedReturn, actualReturn float64) float64 {
	if predictedReturn <= 0 {
		return 0
	}

	if actualReturn <= 0 {
		return 0
	}

	return actualReturn / predictedReturn
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
NormalizeConfidence maps raw confidence into [0, 1] against the local fence.
*/
func NormalizeConfidence(rawScore float64, history []float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	fence := ConfidenceFence(history)

	if fence <= 0 {
		return 1
	}

	if rawScore >= fence {
		return 1
	}

	return rawScore / fence
}
