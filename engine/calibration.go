package engine

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
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

	if feedback.Runway > 0 && calibrator.feedbackCount >= minCalibrationSamples() {
		calibrator.halfLife = adaptiveCalibrationHalfLife(feedback.Runway)
	}

	calibrator.feedbackCount++

	if calibrator.feedbackCount == 1 {
		calibrator.scale = sample
		return
	}

	alpha := calibrator.ewmaAlpha()
	calibrator.scale += alpha * (sample - calibrator.scale)
}

func minCalibrationSamples() int {
	minSamples := config.System.MinCalibrationSamples

	if minSamples <= 0 {
		return 12
	}

	return minSamples
}

func adaptiveCalibrationHalfLife(runway time.Duration) time.Duration {
	if runway <= 0 {
		return defaultCalibrationHalfLife
	}

	floor := config.System.CalibrationHalfLifeFloor

	if floor <= 0 {
		floor = 2 * time.Second
	}

	ceiling := config.System.CalibrationHalfLifeCeiling

	if ceiling <= 0 {
		ceiling = 15 * time.Minute
	}

	halfLife := time.Duration(float64(runway) * config.System.CalibrationRunwayFactor)

	if config.System.CalibrationRunwayFactor <= 0 {
		halfLife = runway / 2
	}

	if halfLife < floor {
		return floor
	}

	if halfLife > ceiling {
		return ceiling
	}

	return halfLife
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
Losing samples return zero with ok=true.
*/
func CalibrationStep(predictedReturn, actualReturn float64) (float64, bool) {
	if predictedReturn <= 0 {
		return 0, false
	}

	if actualReturn <= 0 {
		return 0, true
	}

	return actualReturn / predictedReturn, true
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
func NormalizeConfidence(rawScore float64, history []float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	if len(history) < minConfidenceHistory() {
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

func minConfidenceHistory() int {
	minSamples := config.System.MinConfidenceHistory

	if minSamples <= 0 {
		return 4
	}

	return minSamples
}
