package trader

import (
	kbook "github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/market"
	kticker "github.com/theapemachine/symm/kraken/ticker"
)

/*
MarketQuotes combines ticker quotes with live order book depth for paper fills.
*/
type MarketQuotes struct {
	ticker *kticker.Ticker
	book   *kbook.Book
}

/*
NewMarketQuotes wires ticker and book observers into one fill reader.
*/
func NewMarketQuotes(
	ticker *kticker.Ticker,
	book *kbook.Book,
) *MarketQuotes {
	return &MarketQuotes{
		ticker: ticker,
		book:   book,
	}
}

/*
Last returns the latest trade price for one symbol.
*/
func (quotes *MarketQuotes) Last(symbol string) (float64, bool) {
	if quotes.ticker == nil {
		return 0, false
	}

	last, _, _, _, ok := quotes.ticker.Quote(symbol)

	return last, ok
}

/*
Quote returns last, bid, ask, and 24h change percent for one symbol.
*/
func (quotes *MarketQuotes) Quote(
	symbol string,
) (last, bid, ask, changePct float64, ok bool) {
	if quotes.ticker == nil {
		return 0, 0, 0, 0, false
	}

	return quotes.ticker.Quote(symbol)
}

/*
BookDepth returns bid and ask levels for depth-weighted slippage.
*/
func (quotes *MarketQuotes) BookDepth(
	symbol string,
) (bids, asks []market.BookLevel, ok bool) {
	if quotes.book == nil {
		return nil, nil, false
	}

	return quotes.book.Depth(symbol, 0)
}
