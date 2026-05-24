package trader

import (
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
Prediction is one stored forecast waiting for its runway to elapse.
*/
type Prediction struct {
	source          string
	symbol          string
	measurementType engine.MeasurementType
	regime          string
	reason          string
	direction       int
	predictedAt     time.Time
	dueAt           time.Time
	baselineQuote   float64
	expectedReturn  float64
	runway          time.Duration
}

/*
buildPrediction constructs one forecast from the current pair and measurement.
*/
func (state *PairState) buildPrediction(
	now time.Time, measurement engine.Measurement,
) (Prediction, bool) {
	symbol := asset.Symbol(state.pair)

	if symbol == "" || measurement.Source == "" ||
		measurement.ExpectedReturn <= 0 || measurement.Runway <= 0 {
		return Prediction{}, false
	}

	return Prediction{
		source:          measurement.Source,
		symbol:          symbol,
		measurementType: measurement.Type,
		regime:          measurement.Regime,
		reason:          measurement.Reason,
		direction:       measurement.Type.Direction(),
		predictedAt:     now,
		dueAt:           now.Add(measurement.Runway),
		expectedReturn:  measurement.ExpectedReturn,
		runway:          measurement.Runway,
	}, true
}

/*
settle resolves one anchored forecast against the exit quote.
*/
func (prediction *Prediction) settle(exitQuote float64, settledAt time.Time) engine.PredictionFeedback {
	actualReturn := prediction.signedActualReturn(exitQuote)

	return engine.PredictionFeedback{
		Source:          prediction.source,
		Symbol:          prediction.symbol,
		Type:            prediction.measurementType,
		Regime:          prediction.regime,
		Reason:          prediction.reason,
		PredictedReturn: prediction.expectedReturn,
		ActualReturn:    actualReturn,
		Error:           prediction.expectedReturn - actualReturn,
		Runway:          prediction.runway,
		SettledAt:       settledAt,
	}
}

/*
settleUnanchored records a matured forecast that never received a baseline quote.
*/
func (prediction *Prediction) settleUnanchored(settledAt time.Time) engine.PredictionFeedback {
	return engine.PredictionFeedback{
		Source:          prediction.source,
		Symbol:          prediction.symbol,
		Type:            prediction.measurementType,
		Regime:          prediction.regime,
		Reason:          prediction.reason,
		PredictedReturn: prediction.expectedReturn,
		Runway:          prediction.runway,
		SettledAt:       settledAt,
		Unanchored:      true,
	}
}

/*
signedActualReturn applies forecast direction to the baseline-to-exit move.
*/
func (prediction *Prediction) signedActualReturn(exitQuote float64) float64 {
	if prediction.baselineQuote <= 0 || exitQuote <= 0 || prediction.direction == 0 {
		return 0
	}

	unsignedReturn := (exitQuote - prediction.baselineQuote) / prediction.baselineQuote

	return float64(prediction.direction) * unsignedReturn
}
