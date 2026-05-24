package engine

import "time"

/*
PredictionFeedback is the realized error once a stored prediction matures.
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
FeedbackReceiver ingests settled prediction errors from the trader.
*/
type FeedbackReceiver interface {
	ApplyFeedback(feedback PredictionFeedback)
}
