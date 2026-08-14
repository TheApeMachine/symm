package kraken

import "github.com/krakenfx/api-go/v2/pkg/spot"

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

/*
MarketProfile records the venue rules and active account fee tier used for one
symbol. Captured profiles let replay use the same execution economics as live.
*/
type MarketProfile struct {
	Symbol string         `json:"symbol"`
	Pair   spot.AssetPair `json:"pair"`
	Taker  TradeVolumeFee `json:"taker"`
	Maker  TradeVolumeFee `json:"maker"`
}
