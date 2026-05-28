package broker

import (
	"fmt"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Quote is one market snapshot used to price simulated fills.
*/
type Quote struct {
	Last     float64
	Bid      float64
	Ask      float64
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

	fillPrice := config.System.SlippageFill(
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

	if last <= 0 {
		last = bid

		if ask > 0 {
			last = ask
		}
	}

	if bid <= 0 {
		bid = last
	}

	if ask <= 0 {
		ask = last
	}

	return last, bid, ask, nil
}
