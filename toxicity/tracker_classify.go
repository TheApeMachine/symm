package toxicity

import (
	"math"
	"time"
)

// classifyRemovalLocked splits a removed quantity into fill vs cancel by joining
// the public trade tape, then flags a large, young, near-touch cancel as toxic.
func (tracker *Tracker) classifyRemovalLocked(
	state *symbolState, side byte, price, qty float64, addTs, now time.Time,
) {
	matched := 0.0
	cutoff := now.Add(-tradeMatchWindow)

	for _, trade := range state.trades {
		if trade.at.Before(cutoff) {
			continue
		}

		if math.Abs(trade.price-price)/price <= priceMatchTol {
			matched += trade.volume
		}
	}

	if matched >= fillCoverage*qty {
		tracker.addFlowLocked(state, side, qty, 0)

		return
	}

	tracker.addFlowLocked(state, side, 0, qty)

	sideDepth := state.askTotal
	if side == 'b' {
		sideDepth = state.bidTotal
	}

	large := sideDepth > 0 && qty >= largeBlockFrac*sideDepth
	near := state.mid > 0 && math.Abs(price-state.mid)/state.mid <= toxicProximityPct
	young := now.Sub(addTs) <= toxicMaxAge

	if large && near && young {
		tracker.flagToxicLocked(state, price, 0, now)
	}
}

/*
observeLevelChurnLocked tracks near-touch add/delete velocity per price level.
High cancel ratios within flashChurnWindow flag flash spoofing at the touch.
*/
func (tracker *Tracker) observeLevelChurnLocked(
	state *symbolState, side byte, price, addVol, deleteVol float64, now time.Time,
) {
	if price <= 0 || (addVol <= 0 && deleteVol <= 0) {
		return
	}

	key := l2Key{side: side, price: price}
	window := state.churn[key]

	if window == nil || now.Sub(window.started) > flashChurnWindow {
		window = &levelChurnWindow{started: now}
		state.churn[key] = window
	}

	window.addVol += addVol
	window.deleteVol += deleteVol

	if window.addVol <= 0 {
		return
	}

	ratio := window.deleteVol / window.addVol

	if ratio < flashChurnRatioThreshold {
		return
	}

	if state.mid <= 0 || math.Abs(price-state.mid)/state.mid > toxicProximityPct {
		return
	}

	sideDepth := state.askTotal

	if side == 'b' {
		sideDepth = state.bidTotal
	}

	if sideDepth <= 0 || window.addVol < largeBlockFrac*sideDepth {
		return
	}

	tracker.flagToxicLocked(state, price, ratio, now)
}

func (tracker *Tracker) addFlowLocked(state *symbolState, side byte, fill, cancel float64) {
	if side == 'b' {
		state.fillBid += flowAlpha * (fill - state.fillBid)
		state.cancelBid += flowAlpha * (cancel - state.cancelBid)

		return
	}

	state.fillAsk += flowAlpha * (fill - state.fillAsk)
	state.cancelAsk += flowAlpha * (cancel - state.cancelAsk)
}

func (tracker *Tracker) IsToxic(symbol string, price float64, at time.Time) bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.symbols[symbol]
	if state == nil {
		return false
	}

	expiry, ok := state.toxic[price]
	if !ok {
		return false
	}

	if at.After(expiry) {
		delete(state.toxic, price)
		delete(state.toxicChurn, price)

		return false
	}

	return true
}
