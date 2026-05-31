package broker

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Quote is one market snapshot used to price simulated fills. At carries the
exchange event timestamp so freshness gates can reject stale snapshots before
they cost money.
*/
type Quote struct {
	Last     float64
	Bid      float64
	Ask      float64
	At       time.Time
	BidDepth []market.BookLevel
	AskDepth []market.BookLevel
}

/*
FillPrice returns a depth-aware simulated fill for one side and quote notional.
*/
func (quote *Quote) FillPrice(side string, quoteNotional float64) (float64, error) {
	last, bid, ask, err := quote.complete()

	if err != nil {
		return 0, err
	}

	fillPrice := market.SlippageFill(
		last, bid, ask, side, config.System.SlippageBPS, quoteNotional, quote.BidDepth, quote.AskDepth,
	)

	if fillPrice <= 0 {
		return 0, fmt.Errorf("invalid %s fill price", side)
	}

	return fillPrice, nil
}

func (quote *Quote) complete() (float64, float64, float64, error) {
	last := quote.Last
	bid := quote.Bid
	ask := quote.Ask

	if last <= 0 && bid <= 0 && ask <= 0 {
		return 0, 0, 0, fmt.Errorf("missing quote")
	}

	if bid <= 0 || ask <= 0 || ask < bid {
		return 0, 0, 0, fmt.Errorf("incomplete top of book")
	}

	if last <= 0 {
		last = (bid + ask) / 2
	}

	return last, bid, ask, nil
}

/*
HasTopOfBook reports whether bid and ask are present for spread and slippage gates.
*/
func (quote *Quote) HasTopOfBook() bool {
	return quote.Bid > 0 && quote.Ask > 0 && quote.Ask >= quote.Bid
}
