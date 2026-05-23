package hawkes

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

func resolveMarketObservers(
	observers []engine.Observer,
) (*kbook.Book, *trades.Trades, *kticker.Ticker, error) {
	var book *kbook.Book
	var tradesObserver *trades.Trades
	var tickerObserver *kticker.Ticker

	for _, observer := range observers {
		switch concrete := observer.(type) {
		case *kbook.Book:
			book = concrete
		case *trades.Trades:
			tradesObserver = concrete
		case *kticker.Ticker:
			tickerObserver = concrete
		}
	}

	return book, tradesObserver, tickerObserver, errnie.Require(map[string]any{
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
	})
}
