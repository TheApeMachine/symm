package strategy

/*
Decision is what Evaluate reports for one symbol on one path edge tick.
Reported is the policy role at the path tip; Outcome is set only when the
venue desk accepts enter or exit.
*/
type Decision struct {
	Reported string
	Outcome  string
	PathKey  string
}
