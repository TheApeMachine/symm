package types

import "time"

/*
Signal conditions one market input into numerical measurements. Market
interpretations are deliberately absent because they belong to logic.
*/
type Signal interface {
	Measure(*Thesis) *Thesis
}

/*
InputSignal captures its private market journals at the shared Thesis time so
all signals in one planner cycle measure stable ingress boundaries.
*/
type InputSignal interface {
	Capture(time.Time) error
}
