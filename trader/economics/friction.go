package economics

/*
RoundTripCostPct estimates round-trip friction (fees + slippage) as a fraction
of notional.
*/
func RoundTripCostPct(feePct, slippageBPS float64) float64 {
	return RoundTripCostPctForFees(feePct, feePct, slippageBPS)
}

/*
RoundTripCostPctForFees estimates round-trip friction when entry and exit use
different fee schedules, such as maker entry and taker exit.
*/
func RoundTripCostPctForFees(entryFeePct, exitFeePct, slippageBPS float64) float64 {
	return (entryFeePct+exitFeePct)/100 + slippageBPS/10000
}

/*
NetForwardReturn subtracts round-trip friction from a long forward return.
*/
func NetForwardReturn(forwardReturn, roundTripCost float64) float64 {
	return forwardReturn - roundTripCost
}

/*
NetExitReturn is realized long return minus round-trip friction.
*/
func NetExitReturn(entryPrice, exitPrice, roundTripCost float64) float64 {
	if entryPrice <= 0 {
		return 0
	}

	forward := (exitPrice - entryPrice) / entryPrice

	return NetForwardReturn(forward, roundTripCost)
}
