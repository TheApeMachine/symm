package engine

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/symm/numeric/learned"
)

const (
	defaultCalibrationHalfLife = 5 * time.Minute
	defaultCalibrationTick     = 100 * time.Millisecond
	defaultCalibrationRegime   = "default"
)

/*
PredictionCalibrator tracks running actual/predicted return ratios from settled forecasts.
Scale feeds back into signal parameters, not post-hoc confidence output.
*/
type PredictionCalibrator struct {
	forecasts    map[string]*learned.Forecast
	halfLives    map[string]time.Duration
	activeRegime string
	halfLife     time.Duration
	tickInterval time.Duration
	params       CalibrationParams
}

/*
NewPredictionCalibrator returns a neutral calibrator with injected calibration parameters.
*/
func NewPredictionCalibrator(params CalibrationParams) PredictionCalibrator {
	calibrator := PredictionCalibrator{
		forecasts:    make(map[string]*learned.Forecast),
		halfLives:    make(map[string]time.Duration),
		activeRegime: defaultCalibrationRegime,
		halfLife:     defaultCalibrationHalfLife,
		tickInterval: defaultCalibrationTick,
		params:       params,
	}
	calibrator.forecasts[defaultCalibrationRegime] = learned.NewForecast(0.35)
	calibrator.halfLives[defaultCalibrationRegime] = defaultCalibrationHalfLife

	return calibrator
}

/*
Apply records one settled forecast and updates the running EWMA scale.
Half-life adapts to the signal runway when enough samples exist.
*/
func (calibrator *PredictionCalibrator) Apply(feedback PredictionFeedback) {
	if feedback.Unanchored {
		return
	}

	predictedReturn := feedback.PredictedReturn

	if predictedReturn <= 0 {
		return
	}

	regime := CalibrationRegime(feedback.Regime)
	forecast := calibrator.forecastFor(regime)

	if feedback.Runway > 0 && forecast.Updates() >= calibrator.params.minCalibrationSamples() {
		calibrator.halfLives[regime] = calibrator.params.adaptiveHalfLife(feedback.Runway)
	}

	_ = forecast.Absorb(
		predictedReturn,
		feedback.ActualReturn,
		calibrator.ewmaAlpha(regime),
	)
	calibrator.activeRegime = regime
}

/*
Scale returns the current parameter calibration multiplier.
*/
func (calibrator *PredictionCalibrator) Scale() float64 {
	return calibrator.ScaleFor(calibrator.activeRegime)
}

/*
ScaleFor returns the calibration multiplier for one feedback regime.
*/
func (calibrator *PredictionCalibrator) ScaleFor(regime string) float64 {
	return calibrator.forecastFor(CalibrationRegime(regime)).Scale()
}

/*
CalibrationStep maps realized move to a calibration sample in [0, maxSampleRatio].
Wins scale by actual/predicted; losses preserve magnitude via 1+actual/predicted clamped at zero.
*/
func CalibrationStep(predictedReturn, actualReturn float64) (float64, bool) {
	return learned.SampleRatio(predictedReturn, actualReturn)
}

func (calibrator *PredictionCalibrator) ewmaAlpha(regime string) float64 {
	halfLife := calibrator.halfLives[CalibrationRegime(regime)]

	if halfLife <= 0 {
		halfLife = calibrator.halfLife
	}

	if calibrator.tickInterval <= 0 || halfLife <= 0 {
		return 1
	}

	return 1 - math.Exp(-math.Log(2)*calibrator.tickInterval.Seconds()/halfLife.Seconds())
}

func (calibrator *PredictionCalibrator) forecastFor(regime string) *learned.Forecast {
	if calibrator.forecasts == nil {
		calibrator.forecasts = make(map[string]*learned.Forecast)
	}

	if calibrator.halfLives == nil {
		calibrator.halfLives = make(map[string]time.Duration)
	}

	forecast := calibrator.forecasts[regime]

	if forecast == nil {
		forecast = learned.NewForecast(0.35)
		calibrator.forecasts[regime] = forecast
		calibrator.halfLives[regime] = defaultCalibrationHalfLife
	}

	return forecast
}

/*
CalibrationRegime maps empty feedback regimes into an explicit default bucket.
*/
func CalibrationRegime(regime string) string {
	regime = strings.TrimSpace(regime)

	if regime == "" {
		return defaultCalibrationRegime
	}

	return regime
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
