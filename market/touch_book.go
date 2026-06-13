package market

import (
	"sync"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
TouchBook merges L2 book frames into a touch quote for execution readiness.
*/
type TouchBook struct {
	mu           sync.RWMutex
	bids         map[float64]float64
	asks         map[float64]float64
	last         float64
	ready        bool
	lastObserved time.Time
}

func NewTouchBook() *TouchBook {
	return &TouchBook{
		bids: make(map[float64]float64),
		asks: make(map[float64]float64),
	}
}

func (book *TouchBook) ApplyBookUpdate(
	update *krakenmarket.BookUpdate,
	observedAt time.Time,
) bool {
	if update == nil {
		return false
	}

	book.mu.Lock()
	defer book.mu.Unlock()

	if update.Type == "snapshot" {
		book.resetFromLevels(update.Bids, update.Asks)

		if book.ready {
			book.lastObserved = observedAt
		}

		return book.ready
	}

	if !book.ready {
		return false
	}

	applyBookSide(book.bids, update.Bids)
	applyBookSide(book.asks, update.Asks)
	book.ready = len(book.bids) > 0 && len(book.asks) > 0

	if book.ready {
		book.lastObserved = observedAt
	}

	return book.ready
}

func (book *TouchBook) resetFromLevels(
	bids []krakenmarket.BookLevel,
	asks []krakenmarket.BookLevel,
) {
	clearFloatMap(book.bids)
	clearFloatMap(book.asks)
	applyBookSide(book.bids, bids)
	applyBookSide(book.asks, asks)
	book.ready = len(book.bids) > 0 && len(book.asks) > 0
}

func (book *TouchBook) ApplyTrade(
	trade *krakenmarket.TradeUpdate,
	observedAt time.Time,
) {
	if trade == nil || trade.Price <= 0 {
		return
	}

	book.mu.Lock()
	defer book.mu.Unlock()

	book.last = trade.Price

	if book.ready {
		book.lastObserved = observedAt
	}
}

func (book *TouchBook) ApplyTicker(
	ticker *krakenmarket.TickerUpdate,
	observedAt time.Time,
) {
	if ticker == nil {
		return
	}

	book.mu.Lock()
	defer book.mu.Unlock()

	updated := false

	if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
		applyBookSide(book.bids, []krakenmarket.BookLevel{{
			Price: ticker.Bid,
			Qty:   ticker.BidQty,
		}})
		applyBookSide(book.asks, []krakenmarket.BookLevel{{
			Price: ticker.Ask,
			Qty:   ticker.AskQty,
		}})
		book.ready = true
		updated = true
	}

	if ticker.Last > 0 {
		book.last = ticker.Last
	}

	if updated || book.ready {
		book.lastObserved = observedAt
	}
}

func (book *TouchBook) Snapshot(symbol string) (TouchSnapshot, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	bid, bidOK := bestBookPrice(book.bids, true)
	ask, askOK := bestBookPrice(book.asks, false)

	if !book.ready || !bidOK || !askOK || ask <= bid || book.lastObserved.IsZero() {
		return TouchSnapshot{}, false
	}

	return TouchSnapshot{
		Symbol:     symbol,
		Bid:        bid,
		Ask:        ask,
		Last:       book.last,
		ObservedAt: book.lastObserved,
	}, true
}

func applyBookSide(side map[float64]float64, levels []krakenmarket.BookLevel) {
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
