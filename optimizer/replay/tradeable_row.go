package replay

import (
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
TradeableRow builds an explicit bid/ask/book fixture for replay and tune tests.
Production capture rows must carry real quote fields; this never belongs on the live tape path.
*/
func TradeableRow(symbol string, price float64, at time.Time) types.Measurement {
	return types.Measurement{
		Symbol: symbol,
		Last:   price,
		Bid:    price,
		Ask:    price,
		BookBids: []types.BookLevel{
			{Price: price, Qty: 1_000_000},
		},
		BookAsks: []types.BookLevel{
			{Price: price, Qty: 1_000_000},
		},
		At: at,
	}
}

/*
TradeableRowWithSignal is TradeableRow with signal fields copied for reasoning replay fixtures.
*/
func TradeableRowWithSignal(
	symbol string,
	price float64,
	at time.Time,
	snr float64,
	confidence float64,
) types.Measurement {
	measurement := TradeableRow(symbol, price, at)
	measurement.SNR = snr
	measurement.Confidence = confidence

	return measurement
}
