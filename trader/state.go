package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
PairState tracks forecasts and latest signal readings for one symbol.
*/
type PairState struct {
	mu              sync.Mutex
	pair            asset.Pair
	confidence      float64
	expectedReturn  float64
	regime          string
	reason          string
	measurementType engine.MeasurementType
	runway          time.Duration
	predictions     []Prediction
}

/*
NewPairState creates a new pair state.
*/
func NewPairState(pair asset.Pair) *PairState {
	return &PairState{
		pair: pair,
	}
}

/*
Symbol returns the websocket symbol for this pair state.
*/
func (state *PairState) Symbol() string {
	return asset.Symbol(state.pair)
}

/*
Update ingests the latest signal reading for this pair.
*/
func (state *PairState) Update(measurement engine.Measurement) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.confidence = measurement.Confidence
	state.regime = measurement.Regime
	state.reason = measurement.Reason
	state.measurementType = measurement.Type
}

/*
Predict returns expected return and runway for cross-pair ranking.
*/
func (state *PairState) Predict() (float64, time.Duration) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.expectedReturn <= 0 || state.runway <= 0 {
		return 0, 0
	}

	return state.expectedReturn, state.runway
}

/*
ApplyForecast stores the trader-derived profit forecast for this pair.
*/
func (state *PairState) ApplyForecast(forecast SignalForecast) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.expectedReturn = forecast.ExpectedReturn
	state.runway = forecast.Runway
}

/*
RecordPrediction stores or replaces the open forecast for one source on this symbol.
baselineQuote anchors settlement at signal fire time when positive.
*/
func (state *PairState) RecordPrediction(
	now time.Time,
	measurement engine.Measurement,
	forecast SignalForecast,
	baselineQuote float64,
) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if measurement.Source == "" || forecast.Runway <= 0 {
		return false
	}

	if !forecast.CalibrationOnly && forecast.ExpectedReturn <= 0 {
		return false
	}

	state.predictions = state.dropOpenForecast(measurement.Source, now)

	prediction, ok := state.buildPrediction(now, measurement, forecast, baselineQuote)

	if !ok {
		return false
	}

	state.predictions = append(state.predictions, prediction)

	return true
}

/*
RecordCalibrationProbe stores one non-executable return sample for cold models.
Existing open probes are kept until due so calibration can actually mature.
*/
func (state *PairState) RecordCalibrationProbe(
	now time.Time,
	measurement engine.Measurement,
	runway time.Duration,
	baselineQuote float64,
) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if measurement.Source == "" || runway <= 0 || baselineQuote <= 0 {
		return false
	}

	for _, prediction := range state.predictions {
		if prediction.source == measurement.Source && now.Before(prediction.dueAt) {
			return false
		}
	}

	prediction, ok := state.buildPrediction(
		now,
		measurement,
		SignalForecast{Runway: runway, CalibrationOnly: true},
		baselineQuote,
	)

	if !ok {
		return false
	}

	state.predictions = append(state.predictions, prediction)

	return true
}

/*
AnchorPending attaches the current market quote to every unanchored forecast.
*/
func (state *PairState) AnchorPending(quotePrice float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if quotePrice <= 0 {
		return
	}

	for index := range state.predictions {
		if state.predictions[index].baselineQuote <= 0 {
			state.predictions[index].baselineQuote = quotePrice
		}
	}
}

/*
SettleDue resolves matured forecasts against the current quote.
Unanchored forecasts emit explicit unanchored feedback once due.
*/
func (state *PairState) SettleDue(
	now time.Time, exitQuote float64,
) []engine.PredictionFeedback {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.predictions) == 0 {
		return nil
	}

	feedback := make([]engine.PredictionFeedback, 0, len(state.predictions))
	remaining := state.predictions[:0]

	for _, prediction := range state.predictions {
		settled, keep := state.settleOne(prediction, now, exitQuote)

		if settled != nil {
			feedback = append(feedback, *settled)
		}

		if keep {
			remaining = append(remaining, prediction)
		}
	}

	state.predictions = remaining

	return feedback
}

/*
HasPendingPredictions reports whether unresolved forecasts remain for this pair.
*/
func (state *PairState) HasPendingPredictions() bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	return len(state.predictions) > 0
}

/*
PendingCount returns the number of unresolved forecasts.
*/
func (state *PairState) PendingCount() int {
	state.mu.Lock()
	defer state.mu.Unlock()

	return len(state.predictions)
}

/*
ForecastMetrics returns the latest expected return and average running error
for anchored open forecasts against quotePrice.
*/
func (state *PairState) ForecastMetrics(
	quotePrice float64,
) (prediction, runningError float64, hasPrediction, hasError bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.expectedReturn <= 0 {
		return 0, 0, false, false
	}

	prediction = state.expectedReturn
	hasPrediction = true

	if quotePrice <= 0 || len(state.predictions) == 0 {
		return prediction, 0, true, false
	}

	errorSum := 0.0
	errorCount := 0

	for index := range state.predictions {
		pending := state.predictions[index]

		if pending.baselineQuote <= 0 {
			continue
		}

		actualReturn := pending.signedActualReturn(quotePrice)
		errorSum += pending.expectedReturn - actualReturn
		errorCount++
	}

	if errorCount == 0 {
		return prediction, 0, true, false
	}

	return prediction, errorSum / float64(errorCount), true, true
}

func (state *PairState) dropOpenForecast(source string, now time.Time) []Prediction {
	remaining := state.predictions[:0]

	for _, prediction := range state.predictions {
		if prediction.source == source && now.Before(prediction.dueAt) {
			continue
		}

		remaining = append(remaining, prediction)
	}

	return remaining
}

func (state *PairState) settleOne(
	prediction Prediction, now time.Time, exitQuote float64,
) (*engine.PredictionFeedback, bool) {
	if now.Before(prediction.dueAt) {
		return nil, true
	}

	if prediction.baselineQuote <= 0 {
		feedback := prediction.settleUnanchored(now)

		return &feedback, false
	}

	if exitQuote <= 0 {
		return nil, true
	}

	feedback := prediction.settle(exitQuote, now)

	return &feedback, false
}
