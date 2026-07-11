package strategy

import "time"

/*
Decision records the evaluation of action alternatives and the selected action.
It is appended to the Thesis decision journal.
*/
type Decision struct {
	At           time.Time
	Symbol       string
	Action       Action
	Utility      float64
	Alternatives map[Action]float64
	Reason       string
}
