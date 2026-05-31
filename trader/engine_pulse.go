package trader

import (
	"math"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

func (crypto *Crypto) publishEnginePulse() {
	if crypto.ui == nil {
		return
	}

	snapshot := crypto.ensureCrossSection()
	avgError, avgErrorMultiple := crypto.pulseForecastError(
		snapshot.AvgPredictionMult, snapshot.AvgRequiredReturn,
	)
	phase := "scan"

	if crypto.open.Load() > 0 {
		phase = "trade"
	}

	frame := map[string]any{
		"event":                   "engine_pulse",
		"ts":                      time.Now().UTC().Format(time.RFC3339Nano),
		"seq":                     crypto.pulseSeq.Add(1),
		"phase":                   phase,
		"measurements":            snapshot.Measurements,
		"candidates":              snapshot.Candidates,
		"open":                    crypto.open.Load(),
		"ticker_ready":            crypto.quotes.readyCount(),
		"symbols_total":           len(crypto.scopedRuntime().Signal.Symbols),
		"fluid_sampled":           crypto.fluidSampleCount(),
		"avg_prediction":          snapshot.AvgPrediction,
		"avg_prediction_multiple": snapshot.AvgPredictionMult,
		"avg_required_return":     snapshot.AvgRequiredReturn,
		"avg_error":               avgError,
		"avg_error_multiple":      avgErrorMultiple,
		"forecast_symbols":        snapshot.ForecastSymbols,
		"scaled_forecast_symbols": snapshot.ForecastSymbols,
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: frame})
}

/*
pulseForecastError is the catch-up miss when the prior pulse's forward forecast
(targeting this instant) meets the newly realized cross-section multiple.
*/
func (crypto *Crypto) pulseForecastError(currentMultiple, avgRequired float64) (float64, float64) {
	forecastError := 0.0

	if crypto.priorPulseMultipleValid {
		forecastError = math.Abs(currentMultiple - crypto.priorPulseMultiple)
	}

	crypto.priorPulseMultiple = currentMultiple
	crypto.priorPulseMultipleValid = true

	forecastErrorMultiple := 0.0

	if avgRequired > 0 {
		forecastErrorMultiple = forecastError / avgRequired
	}

	return forecastError, forecastErrorMultiple
}

func (crypto *Crypto) fluidSampleCount() int {
	now := time.Now()
	count := 0

	crypto.mu.RLock()
	defer crypto.mu.RUnlock()

	for _, set := range crypto.readings {
		slot, ok := set[perspectives.SourceFluid]

		if !ok || slot.Stale(now) {
			continue
		}

		count++
	}

	return count
}
