package trader

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
)

func (crypto *Crypto) emitEnginePulse(phase string) {
	if crypto == nil {
		return
	}

	group := crypto.broadcasts["ui"]

	if group == nil {
		errnie.Error(fmt.Errorf("ui broadcast missing for engine pulse"))
		return
	}

	group.Send(&qpool.QValue[any]{Value: crypto.enginePulse(phase)})
}

func (crypto *Crypto) enginePulse(phase string) map[string]any {
	now := time.Now()
	scale := crypto.averageOpenPrediction(now)
	avgError := 0.0

	if crypto.forecasts != nil {
		avgError = crypto.forecasts.RunningMeanError()
	} else {
		errnie.Error(fmt.Errorf("forecast model missing for engine pulse"))
	}

	pulse := map[string]any{
		"event":            "engine_pulse",
		"seq":              crypto.pulseSeq.Add(1),
		"phase":            phase,
		"measurements":     crypto.activeMeasurementCount(now),
		"candidates":       crypto.perspectiveCandidateCount(),
		"open":             crypto.openCount(),
		"avg_prediction":   scale.averagePrediction(),
		"avg_error":        avgError,
		"forecast_symbols": scale.forecastSymbols,
		"ts":               now.UTC().Format(time.RFC3339Nano),
	}

	if scale.scaledForecastSymbols > 0 {
		pulse["avg_prediction_multiple"] = scale.averagePredictionMultiple()
		pulse["avg_required_return"] = scale.averageRequiredReturn()
		pulse["avg_error_multiple"] = avgError / scale.averageRequiredReturn()
		pulse["scaled_forecast_symbols"] = scale.scaledForecastSymbols
	}

	return pulse
}

func (crypto *Crypto) activeMeasurementCount(now time.Time) int {
	if crypto == nil {
		return 0
	}

	crypto.perspectivesMu.RLock()
	perspectives := make([]*Perspective, 0, len(crypto.perspectives))

	for _, perspective := range crypto.perspectives {
		if perspective != nil {
			perspectives = append(perspectives, perspective)
		}
	}

	crypto.perspectivesMu.RUnlock()

	count := 0

	for _, perspective := range perspectives {
		count += perspective.activeMeasurementCount(now)
	}

	return count
}

func (crypto *Crypto) perspectiveCandidateCount() int {
	if crypto == nil {
		return 0
	}

	crypto.perspectivesMu.RLock()
	defer crypto.perspectivesMu.RUnlock()

	return len(crypto.perspectives)
}

type enginePulsePredictionScale struct {
	predictionSum         float64
	predictionMultipleSum float64
	requiredReturnSum     float64
	forecastSymbols       int
	scaledForecastSymbols int
}

func (crypto *Crypto) averageOpenPrediction(
	now time.Time,
) enginePulsePredictionScale {
	scale := enginePulsePredictionScale{}

	for _, prediction := range crypto.predictions {
		if prediction == nil || prediction.DueAt.Before(now) {
			continue
		}

		if math.IsNaN(prediction.ExpectedReturn) ||
			math.IsInf(prediction.ExpectedReturn, 0) {
			continue
		}

		scale.predictionSum += prediction.ExpectedReturn
		scale.forecastSymbols++

		requirement, ok := crypto.predictionReturnRequirement(prediction)

		if !ok {
			continue
		}

		multiple, ok := requirement.multiple(prediction.ExpectedReturn)

		if !ok {
			continue
		}

		scale.predictionMultipleSum += multiple
		scale.requiredReturnSum += requirement.requiredReturn
		scale.scaledForecastSymbols++
	}

	return scale
}

func (scale enginePulsePredictionScale) averagePrediction() float64 {
	if scale.forecastSymbols == 0 {
		return 0
	}

	return scale.predictionSum / float64(scale.forecastSymbols)
}

func (scale enginePulsePredictionScale) averagePredictionMultiple() float64 {
	if scale.scaledForecastSymbols == 0 {
		return 0
	}

	return scale.predictionMultipleSum / float64(scale.scaledForecastSymbols)
}

func (scale enginePulsePredictionScale) averageRequiredReturn() float64 {
	if scale.scaledForecastSymbols == 0 {
		return 0
	}

	return scale.requiredReturnSum / float64(scale.scaledForecastSymbols)
}

func (crypto *Crypto) predictionReturnRequirement(
	prediction *engine.Prediction,
) (entryReturnRequirement, bool) {
	if prediction == nil {
		return entryReturnRequirement{}, false
	}

	lead, ok := prediction.LeadMeasurement()

	if !ok {
		return entryReturnRequirement{}, false
	}

	return crypto.entryReturnRequirement(lead.Pairs[0].Wsname, lead), true
}
