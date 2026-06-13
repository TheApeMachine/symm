package market

import (
	"sync/atomic"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

type touchBookState struct {
	bids         map[float64]float64
	asks         map[float64]float64
	last         float64
	ready        bool
	lastObserved time.Time
}

/*
TouchBook merges L2 book frames into a touch quote for execution readiness.
*/
type TouchBook struct {
	state atomic.Pointer[touchBookState]
}

func NewTouchBook() *TouchBook {
	book := &TouchBook{}
	book.state.Store(&touchBookState{
		bids: make(map[float64]float64),
		asks: make(map[float64]float64),
	})

	return book
}

func (book *TouchBook) ApplyBookUpdate(
	update *krakenmarket.BookUpdate,
	observedAt time.Time,
) bool {
	if update == nil {
		return false
	}

	var ready bool

	book.swapState(func(state *touchBookState) {
		if update.Type == "snapshot" {
			resetTouchBookFromLevels(state, update.Bids, update.Asks)

			if state.ready {
				state.lastObserved = observedAt
			}

			ready = state.ready

			return
		}

		if !state.ready {
			return
		}

		applyBookSide(state.bids, update.Bids)
		applyBookSide(state.asks, update.Asks)
		state.ready = len(state.bids) > 0 && len(state.asks) > 0

		if state.ready {
			state.lastObserved = observedAt
		}

		ready = state.ready
	})

	return ready
}

func (book *TouchBook) ApplyTrade(
	trade *krakenmarket.TradeUpdate,
	observedAt time.Time,
) {
	if trade == nil || trade.Price <= 0 {
		return
	}

	book.swapState(func(state *touchBookState) {
		state.last = trade.Price

		if state.ready {
			state.lastObserved = observedAt
		}
	})
}

func (book *TouchBook) ApplyTicker(
	ticker *krakenmarket.TickerUpdate,
	observedAt time.Time,
) {
	if ticker == nil {
		return
	}

	book.swapState(func(state *touchBookState) {
		updated := false

		if ticker.Bid > 0 && ticker.Ask > ticker.Bid {
			applyBookSide(state.bids, []krakenmarket.BookLevel{{
				Price: ticker.Bid,
				Qty:   ticker.BidQty,
			}})
			applyBookSide(state.asks, []krakenmarket.BookLevel{{
				Price: ticker.Ask,
				Qty:   ticker.AskQty,
			}})
			state.ready = true
			updated = true
		}

		if ticker.Last > 0 {
			state.last = ticker.Last
		}

		if updated || state.ready {
			state.lastObserved = observedAt
		}
	})
}

func (book *TouchBook) Snapshot(symbol string) (TouchSnapshot, bool) {
	state := book.state.Load()

	if state == nil {
		return TouchSnapshot{}, false
	}

	bid, bidOK := bestBookPrice(state.bids, true)
	ask, askOK := bestBookPrice(state.asks, false)

	if !state.ready || !bidOK || !askOK || ask <= bid || state.lastObserved.IsZero() {
		return TouchSnapshot{}, false
	}

	return TouchSnapshot{
		Symbol:     symbol,
		Bid:        bid,
		Ask:        ask,
		Last:       state.last,
		ObservedAt: state.lastObserved,
	}, true
}

func (book *TouchBook) swapState(mutate func(*touchBookState)) {
	for {
		current := book.state.Load()
		next := cloneTouchBookState(current)
		mutate(next)

		if book.state.CompareAndSwap(current, next) {
			return
		}
	}
}

func cloneTouchBookState(state *touchBookState) *touchBookState {
	if state == nil {
		return &touchBookState{
			bids: make(map[float64]float64),
			asks: make(map[float64]float64),
		}
	}

	bids := make(map[float64]float64, len(state.bids))

	for price, quantity := range state.bids {
		bids[price] = quantity
	}

	asks := make(map[float64]float64, len(state.asks))

	for price, quantity := range state.asks {
		asks[price] = quantity
	}

	return &touchBookState{
		bids:         bids,
		asks:         asks,
		last:         state.last,
		ready:        state.ready,
		lastObserved: state.lastObserved,
	}
}

func resetTouchBookFromLevels(
	state *touchBookState,
	bids []krakenmarket.BookLevel,
	asks []krakenmarket.BookLevel,
) {
	clearFloatMap(state.bids)
	clearFloatMap(state.asks)
	applyBookSide(state.bids, bids)
	applyBookSide(state.asks, asks)
	state.ready = len(state.bids) > 0 && len(state.asks) > 0
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
