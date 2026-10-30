package strategy

/*
entryNets are the grid vectors used at enter time (story + board confidence),
aligned with EvaluateEnter and the gate training frame.
*/
func entryNets(b [15]float64) (netSpark, netHold float64) {
	netSpark = (b[0] + b[1] + b[9]) - b[2]
	netHold = (b[6] + b[7] + b[10]) - b[8]
	return netSpark, netHold
}
