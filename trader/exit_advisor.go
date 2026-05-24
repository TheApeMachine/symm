package trader

/*
ExitAdvisor scores how urgently an open position should be closed.
*/
type ExitAdvisor interface {
	ExitUrgency(symbol string, side int) (urgency float64, reason string)
}
