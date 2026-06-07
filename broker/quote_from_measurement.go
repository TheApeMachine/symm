package broker

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
QuoteFromMeasurement builds a broker.Quote for preflight and fill simulation from
a tape row. Optimizer replay and tests use this so slippage math matches the desk.
*/
func QuoteFromMeasurement(measurement types.Measurement) Quote {
	quote := Quote{
		Symbol: measurement.Symbol,
		Bid:    measurement.Bid,
		Ask:    measurement.Ask,
		Last:   measurement.Last,
	}

	if !measurement.At.IsZero() {
		quote.UpdatedAt = measurement.At
	}

	if measurement.HasBookDepth() {
		quote.Book = measurement.MarketBook()
	}

	if quote.Bid <= 0 && quote.Ask <= 0 {
		bid, ask, ok := DeriveBidAsk(measurement)

		if ok {
			quote.Bid = bid
			quote.Ask = ask
		}
	}

	if quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = time.Now().UTC()
	}

	if quote.Book.Bids == nil && quote.Book.Asks == nil {
		quote.Book = market.Book{}
	}

	return quote
}
