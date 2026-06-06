package toxicity

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

// ApplyOrder ingests one L3 event. event is "add", "delete", or "amend"; ts is
// the order's matching-engine timestamp from the level3 message.
func (tracker *Tracker) ApplyOrder(
	symbol string, pair market.Pair, event, orderID string,
	side byte, price, qty float64, ts, now time.Time,
) {
	if orderID == "" {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.stateLocked(symbol, pair)

	switch event {
	case "add":
		if _, exists := state.orders[orderID]; exists {
			return
		}

		state.orders[orderID] = &orderState{side: side, price: price, qty: qty, addTs: ts}
		state.addDepth(side, qty)
		tracker.observeLevelChurnLocked(state, side, price, qty, 0, now)

	case "delete":
		order := state.orders[orderID]
		if order == nil {
			return
		}

		tracker.observeLevelChurnLocked(state, order.side, order.price, 0, order.qty, now)
		tracker.classifyRemovalLocked(state, order.side, order.price, order.qty, order.addTs, now)
		state.addDepth(order.side, -order.qty)
		delete(state.orders, orderID)

	case "modify", "amend":
		order := state.orders[orderID]

		if order == nil {
			state.orders[orderID] = &orderState{side: side, price: price, qty: qty, addTs: ts}
			state.addDepth(side, qty)

			return
		}

		// A price change is remove+add; a quantity cut at the same price is a
		// partial removal of the delta, joined to trades like any removal.
		if price != order.price {
			tracker.observeLevelChurnLocked(state, order.side, order.price, 0, order.qty, now)
			tracker.classifyRemovalLocked(state, order.side, order.price, order.qty, order.addTs, now)
			state.addDepth(order.side, -order.qty)
			order.side, order.price, order.qty, order.addTs = side, price, qty, ts
			state.addDepth(side, qty)
			tracker.observeLevelChurnLocked(state, side, price, qty, 0, now)

			return
		}

		if delta := qty - order.qty; delta < 0 {
			tracker.observeLevelChurnLocked(state, order.side, order.price, 0, -delta, now)
			tracker.classifyRemovalLocked(state, order.side, order.price, -delta, order.addTs, now)
		}

		state.addDepth(order.side, qty-order.qty)
		order.qty = qty
	}
}
