package engine

import (
	"iter"
)

/*
Signal emits regime measurements and ingests settled prediction feedback.
*/
type Signal interface {
	Source() string
	Measure() iter.Seq[Measurement]
	Feedback(feedback PredictionFeedback)
}
