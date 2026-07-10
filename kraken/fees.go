package kraken

/*
FeeRates carries taker and maker fee rates for one pair as fractions.
*/
type FeeRates struct {
	Taker float64
	Maker float64
}

/*
FeeSchedule is the TradeVolume lookup keyed by websocket symbol.
*/
type FeeSchedule struct {
	Pairs map[string]FeeRates
}
