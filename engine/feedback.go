package engine

import "time"

/*
PredictionFeedback is the realized error once a stored prediction matures.
The trader emits this after Prediction.DueAt; each Signal.Feedback ingests it
and updates a learned.Forecast scale (directly or via PredictionCalibrator).
*/
type PredictionFeedback struct {
	Source          string
	Symbol          string
	Type            MeasurementType
	Regime          string
	Reason          string
	Confidence      float64
	PredictedReturn float64
	ActualReturn    float64
	Error           float64
	Runway          time.Duration
	SettledAt       time.Time
	Unanchored      bool
}

/*
ValidPredictionFeedback reports whether settled feedback should be emitted to signals.
Unanchored or zero predicted-return feedback is dropped — no silent defaults.
*/
func ValidPredictionFeedback(feedback PredictionFeedback) bool {
	if feedback.Source == "" || feedback.Symbol == "" {
		return false
	}

	if feedback.Unanchored || feedback.PredictedReturn <= 0 {
		return false
	}

	return true
}
