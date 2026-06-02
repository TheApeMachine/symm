package toxicity

import (
	"math"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

// ApplyBookLevel ingests one L2 aggregated book level (§16.4.4 fallback). qty
// is the new absolute resting quantity at (side, price); qty <= 0 removes the
// level. A decrement is joined to the trade tape exactly like an L3 removal,
// keyed by price level with the level's first-seen time as the age proxy.
func (tracker *Tracker) ApplyBookLevel(
	symbol string, pair market.Pair, side byte, price, qty float64, now time.Time,
) {
	if price <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.stateLocked(symbol, pair)
	key := l2Key{side: side, price: price}
	level := state.levels[key]

	prevQty := 0.0
	firstSeen := now

	if level != nil {
		prevQty = level.qty
		firstSeen = level.firstSeen
	}

	switch {
	case qty <= 0:
		// Level cleared: the whole resting quantity was removed.
		if prevQty > 0 {
			state.addDepth(side, -prevQty)
			tracker.classifyRemovalLocked(state, side, price, prevQty, firstSeen, now)
		}

		delete(state.levels, key)

	case qty > prevQty:
		// Level grew (a fresh add or refill).
		state.addDepth(side, qty-prevQty)
		tracker.observeLevelChurnLocked(state, side, price, qty-prevQty, 0, now)

		if level == nil {
			state.levels[key] = &l2Level{qty: qty, firstSeen: now}

			return
		}

		level.qty = qty

	case qty < prevQty:
		// Level shrank: classify the removed delta.
		state.addDepth(side, qty-prevQty)
		tracker.observeLevelChurnLocked(state, side, price, 0, prevQty-qty, now)
		tracker.classifyRemovalLocked(state, side, price, prevQty-qty, firstSeen, now)
		level.qty = qty
	}
}

func (state *symbolState) addDepth(side byte, delta float64) {
	if side == 'b' {
		state.bidTotal = math.Max(0, state.bidTotal+delta)

		return
	}

	state.askTotal = math.Max(0, state.askTotal+delta)
}
