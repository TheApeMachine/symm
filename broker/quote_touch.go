package broker

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

/*
symbolBook merges L2 book frames into a touch quote for pre-trade risk.
*/
type symbolBook struct {
	bids  map[float64]float64
	asks  map[float64]float64
	last  float64
	ready bool
}

func newSymbolBook() *symbolBook {
	return &symbolBook{
		bids: make(map[float64]float64),
		asks: make(map[float64]float64),
	}
}

func (book *symbolBook) applyBookUpdate(update *market.BookUpdate) bool {
	if update == nil {
		return false
	}

	if update.Type == "snapshot" {
		book.resetFromLevels(update.Bids, update.Asks)

		return book.ready
	}

	if !book.ready {
		return false
	}

	applyBookSide(book.bids, update.Bids)
	applyBookSide(book.asks, update.Asks)
	book.ready = len(book.bids) > 0 && len(book.asks) > 0

	return book.ready
}

func (book *symbolBook) resetFromLevels(
	bids []market.BookLevel,
	asks []market.BookLevel,
) {
	clearFloatMap(book.bids)
	clearFloatMap(book.asks)
	applyBookSide(book.bids, bids)
	applyBookSide(book.asks, asks)
	book.ready = len(book.bids) > 0 && len(book.asks) > 0
}

func (book *symbolBook) applyTrade(trade *market.TradeUpdate) {
	if trade == nil || trade.Price <= 0 {
		return
	}

	book.last = trade.Price
}

func (book *symbolBook) applyTicker(ticker *market.TickerUpdate) {
	if ticker == nil {
		return
	}

	if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
		applyBookSide(book.bids, []market.BookLevel{{
			Price: ticker.Bid,
			Qty:   ticker.BidQty,
		}})
		applyBookSide(book.asks, []market.BookLevel{{
			Price: ticker.Ask,
			Qty:   ticker.AskQty,
		}})
		book.ready = true
	}

	if ticker.Last > 0 {
		book.last = ticker.Last
	}
}

func (book *symbolBook) quoteSnapshot(
	symbol string,
	observedAt time.Time,
) (QuoteSnapshot, bool) {
	bid, bidOK := bestBookPrice(book.bids, true)
	ask, askOK := bestBookPrice(book.asks, false)

	if !book.ready || !bidOK || !askOK || ask <= bid {
		return QuoteSnapshot{}, false
	}

	return QuoteSnapshot{
		Symbol:     symbol,
		Bid:        bid,
		Ask:        ask,
		Last:       book.last,
		ObservedAt: observedAt,
	}, true
}

func applyBookSide(side map[float64]float64, levels []market.BookLevel) {
	for _, level := range levels {
		if level.Qty <= 0 {
			delete(side, level.Price)
			continue
		}

		side[level.Price] = level.Qty
	}
}

func bestBookPrice(side map[float64]float64, bidSide bool) (float64, bool) {
	best := 0.0
	found := false

	for price, quantity := range side {
		if quantity <= 0 {
			continue
		}

		if !bidSide {
			if !found || price < best {
				best = price
				found = true
			}

			continue
		}

		if !found || price > best {
			best = price
			found = true
		}
	}

	return best, found
}

func clearFloatMap(values map[float64]float64) {
	for key := range values {
		delete(values, key)
	}
}
