package engine

import "time"

/*
PredictionFeedback is the realized error once a stored prediction matures.
The trader emits this after Prediction.DueAt; each Signal.Feedback ingests it
and updates a learned.Forecast scale (directly or via PredictionCalibrator).
*/
type PredictionFeedback struct {
	Source          string
	Sources         []string
	Symbol          string
	Type            MeasurementType
	PerspectiveType PerspectiveType
	Regime          string
	Reason          string
	Confidence      float64
	PredictedReturn float64
	ActualReturn    float64
	Error           float64
	Runway          time.Duration
	PredictedAt     time.Time
	DueAt           time.Time
	SettledAt       time.Time
	Unanchored      bool
}

/*
ValidPredictionFeedback reports whether settled feedback should be emitted to signals.
Only unanchored or incomplete rows are dropped.
*/
func ValidPredictionFeedback(feedback PredictionFeedback) bool {
	if feedback.Source == "" || feedback.Symbol == "" {
		return false
	}

	return !feedback.Unanchored
}

/*
FeedbackIncludesSource reports whether a signal source contributed to a
perspective-level prediction feedback payload.
*/
func FeedbackIncludesSource(feedback PredictionFeedback, source string) bool {
	if source == "" {
		return false
	}

	if feedback.Source == source {
		return true
	}

	for _, candidate := range feedback.Sources {
		if candidate == source {
			return true
		}
	}

	return false
}
