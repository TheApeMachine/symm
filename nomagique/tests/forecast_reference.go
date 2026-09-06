// Test-only copy of the supplied learning/forecast.go implementation.
package tests

import (
	"math"

	"fmt"
)

/*
referenceCalibrationForecastOutput reports learned multiplicative scale.
*/
type referenceCalibrationForecastOutput struct {
	Value       float64
	Predicted   float64
	Actual      float64
	Scale       float64
	Trust       float64
	Rate        float64
	Count       int
	WeightCount int
}

/*
referenceCalibrationForecaster learns a multiplicative scale from predicted-vs-actual outcomes.
Residual mean and variance are tracked online so a zero residual span is a
valid zero-surprise condition rather than a validation failure.
*/
type referenceCalibrationForecaster struct {
	scale       float64
	trust       float64
	rate        float64
	mean        float64
	m2          float64
	weightCount int
	count       int
}

/*
referenceCalibrationForecast returns a typed scale learner.
*/
func referenceCalibrationForecast() *referenceCalibrationForecaster {
	return &referenceCalibrationForecaster{}
}

/*
Measure updates forecast scale from one prediction outcome.
*/
func (forecaster *referenceCalibrationForecaster) Measure(pair referenceCalibrationLearningPair) (referenceCalibrationForecastOutput, error) {
	predicted, actual, err := referenceCalibrationValidate(pair, "forecast")

	if err != nil {
		return referenceCalibrationForecastOutput{}, err
	}

	residual := actual - predicted

	if forecaster.count == 0 {
		forecaster.scale = 1
		forecaster.trust = 1
		forecaster.mean = residual
		forecaster.m2 = 0
		forecaster.weightCount = 1
		forecaster.count = 1

		return referenceCalibrationForecastOutput{
			Value:       forecaster.scale,
			Predicted:   predicted,
			Actual:      actual,
			Scale:       forecaster.scale,
			Trust:       forecaster.trust,
			Rate:        0,
			Count:       forecaster.count,
			WeightCount: forecaster.weightCount,
		}, nil
	}

	forecaster.weightCount++
	delta := residual - forecaster.mean
	forecaster.mean += delta / float64(forecaster.weightCount)
	forecaster.m2 += delta * (residual - forecaster.mean)

	variance := 0.0

	if forecaster.weightCount > 1 {
		variance = forecaster.m2 / float64(forecaster.weightCount-1)
	}

	if variance < 0 || math.IsNaN(variance) || math.IsInf(variance, 0) {
		return referenceCalibrationForecastOutput{}, fmt.Errorf("forecast: residual variance must be finite and non-negative")
	}

	surprise := 0.0

	if variance > 0 {
		surprise = math.Abs(residual-forecaster.mean) / math.Sqrt(variance)
	}

	forecaster.rate = surprise
	targetTrust := math.Max(0, 1-surprise)
	forecaster.trust += surprise * (targetTrust - forecaster.trust)
	learningRate := surprise * (1 - forecaster.trust)
	magnitudePredicted := math.Abs(predicted)
	magnitudeActual := math.Abs(actual)

	if magnitudePredicted <= 0 {
		return referenceCalibrationForecastOutput{}, fmt.Errorf("forecast: predicted magnitude must be positive for log-ratio scale")
	}

	if magnitudeActual <= 0 {
		return referenceCalibrationForecastOutput{}, fmt.Errorf("forecast: actual magnitude must be positive for log-ratio scale")
	}

	targetScale := math.Exp(math.Log(magnitudeActual) - math.Log(magnitudePredicted))
	forecaster.scale += learningRate * (targetScale - forecaster.scale)
	forecaster.count++

	if !referenceRLSFinite(forecaster.scale) {
		return referenceCalibrationForecastOutput{}, fmt.Errorf("forecast: scale must stay finite")
	}

	return referenceCalibrationForecastOutput{
		Value:       forecaster.scale,
		Predicted:   predicted,
		Actual:      actual,
		Scale:       forecaster.scale,
		Trust:       forecaster.trust,
		Rate:        forecaster.rate,
		Count:       forecaster.count,
		WeightCount: forecaster.weightCount,
	}, nil
}

/*
Reset clears learned forecast state.
*/
func (forecaster *referenceCalibrationForecaster) Reset() {
	*forecaster = referenceCalibrationForecaster{}
}
