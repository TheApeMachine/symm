package engine

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/symm/numeric/learned"
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
	forecast     *learned.Forecast
	halfLife     time.Duration
	tickInterval time.Duration
	params       CalibrationParams
}

/*
NewPredictionCalibrator returns a neutral calibrator with injected calibration parameters.
*/
func NewPredictionCalibrator(params CalibrationParams) PredictionCalibrator {
	return PredictionCalibrator{
		forecast:     learned.NewForecast(0.35),
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

	if feedback.Runway > 0 && calibrator.forecast.Updates() >= calibrator.params.minCalibrationSamples() {
		calibrator.halfLife = calibrator.params.adaptiveHalfLife(feedback.Runway)
	}

	_ = calibrator.forecast.Absorb(
		feedback.PredictedReturn,
		feedback.ActualReturn,
		calibrator.ewmaAlpha(),
	)
}

/*
Scale returns the current parameter calibration multiplier.
*/
func (calibrator *PredictionCalibrator) Scale() float64 {
	return calibrator.forecast.Scale()
}

/*
CalibrationStep maps realized move to a calibration sample in [0, maxSampleRatio].
Wins scale by actual/predicted; losses preserve magnitude via 1+actual/predicted clamped at zero.
*/
func CalibrationStep(predictedReturn, actualReturn float64) (float64, bool) {
	return learned.SampleRatio(predictedReturn, actualReturn)
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

	lower, upper := quartiles(values)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return valuesMax(values)
}

func quartiles(values []float64) (lower, upper float64) {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	n := len(sorted)
	lower = sorted[n/4]
	upper = sorted[(3*n)/4]

	return lower, upper
}

func valuesMax(values []float64) float64 {
	maxValue := values[0]

	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}

	return maxValue
}

/*
NormalizeConfidence maps raw signal strength into (0, 1) against the symbol-local fence.
The fence is the half-saturation point: raw equal to the fence yields 0.5 strength.
Strength saturates via raw/(raw+fence) so no reading — however extreme — implies certainty.
Returns 0 until enough history exists to calibrate; never invents strength on a cold symbol.
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

	return rawScore / (rawScore + fence)
}
