package trader

import "github.com/theapemachine/symm/kraken/market"

/*
PriceReader supplies the latest trade price for prediction settlement.
*/
type PriceReader interface {
	Last(symbol string) (float64, bool)
}

/*
QuoteReader extends PriceReader with bid and ask for paper fills and stops.
*/
type QuoteReader interface {
	PriceReader
	Quote(symbol string) (last, bid, ask, changePct float64, ok bool)
}

/*
FillReader extends QuoteReader with order book depth for VWAP slippage.
*/
type FillReader interface {
	QuoteReader
	BookDepth(symbol string) (bids, asks []market.BookLevel, ok bool)
}
