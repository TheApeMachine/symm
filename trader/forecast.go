package trader

/*
ForecastSnapshot is the cross-symbol average prediction and running error.
*/
type ForecastSnapshot struct {
	AvgPrediction    float64
	AvgError         float64
	PredictedSymbols int
	ErrorSymbols     int
}

/*
resolveForecast averages per-symbol prediction and error from pair states,
current tick readings, or scored evaluations when forecasts are not yet live.
*/
func (crypto *Crypto) resolveForecast(
	readings map[string]symbolReadings,
	line float64,
	evaluations []map[string]any,
) ForecastSnapshot {
	if snapshot := crypto.aggregateForecasts(); snapshot.PredictedSymbols > 0 {
		return snapshot
	}

	if snapshot := crypto.forecastFromReadings(readings); snapshot.PredictedSymbols > 0 {
		return snapshot
	}

	return forecastFromEvaluations(line, evaluations)
}

/*
aggregateForecasts averages expected return and running error from pair states.
*/
func (crypto *Crypto) aggregateForecasts() ForecastSnapshot {
	snapshot := ForecastSnapshot{}
	predictionSum := 0.0
	errorSum := 0.0

	crypto.pairStates.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		state, ok := value.(*PairState)

		if !ok || state == nil {
			return true
		}

		quotePrice, _ := crypto.quotePrice(symbol)
		prediction, runningError, hasPrediction, hasError := state.ForecastMetrics(quotePrice)

		if hasPrediction {
			predictionSum += prediction
			snapshot.PredictedSymbols++
		}

		if hasError {
			errorSum += runningError
			snapshot.ErrorSymbols++
		}

		return true
	})

	if snapshot.PredictedSymbols > 0 {
		snapshot.AvgPrediction = predictionSum / float64(snapshot.PredictedSymbols)
	}

	if snapshot.ErrorSymbols > 0 {
		snapshot.AvgError = errorSum / float64(snapshot.ErrorSymbols)
	}

	return snapshot
}

func (crypto *Crypto) forecastFromReadings(
	readings map[string]symbolReadings,
) ForecastSnapshot {
	snapshot := ForecastSnapshot{}
	predictionSum := 0.0
	errorSum := 0.0

	for symbol, sources := range readings {
		bestReturn := 0.0

		for _, reading := range sources {
			if reading.expectedReturn > bestReturn {
				bestReturn = reading.expectedReturn
			}
		}

		if bestReturn <= 0 {
			continue
		}

		predictionSum += bestReturn
		snapshot.PredictedSymbols++

		state := crypto.pairStateBySymbol(symbol)

		if state == nil {
			continue
		}

		quotePrice, _ := crypto.quotePrice(symbol)
		_, runningError, _, hasError := state.ForecastMetrics(quotePrice)

		if !hasError {
			continue
		}

		errorSum += runningError
		snapshot.ErrorSymbols++
	}

	if snapshot.PredictedSymbols > 0 {
		snapshot.AvgPrediction = predictionSum / float64(snapshot.PredictedSymbols)
	}

	if snapshot.ErrorSymbols > 0 {
		snapshot.AvgError = errorSum / float64(snapshot.ErrorSymbols)
	}

	return snapshot
}

func forecastFromEvaluations(
	line float64,
	evaluations []map[string]any,
) ForecastSnapshot {
	if len(evaluations) == 0 {
		return ForecastSnapshot{}
	}

	predictionSum := 0.0
	errorSum := 0.0
	scored := 0

	for _, row := range evaluations {
		combined, _ := row["combined"].(float64)

		if combined <= 0 {
			continue
		}

		predictionSum += combined
		errorSum += combined - line
		scored++
	}

	if scored == 0 {
		return ForecastSnapshot{}
	}

	return ForecastSnapshot{
		PredictedSymbols: scored,
		ErrorSymbols:     scored,
		AvgPrediction:    predictionSum / float64(scored),
		AvgError:         errorSum / float64(scored),
	}
}

func (crypto *Crypto) pairStateBySymbol(symbol string) *PairState {
	if symbol == "" {
		return nil
	}

	if loaded, ok := crypto.pairStates.Load(symbol); ok {
		return loaded.(*PairState)
	}

	return nil
}
