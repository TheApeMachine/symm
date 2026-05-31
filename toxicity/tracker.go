package toxicity

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

const (
	tradeMatchWindow         = 2 * time.Second
	priceMatchTol            = 0.0002 // tight: L3 gives exact prices
	fillCoverage             = 0.5    // matched trade vol >= this x removed qty => fill
	toxicMaxAge              = 10 * time.Second
	toxicProximityPct        = 0.005 // 0.5 % of mid
	largeBlockFrac           = 0.10  // >= 10 % of that side's visible depth = "large"
	toxicCooldown            = 30 * time.Second
	flowAlpha                = 0.05
	tradeRingCap             = 512
	epsilon                  = 1e-12
	flashChurnWindow         = 50 * time.Millisecond
	flashChurnRatioThreshold = 0.85
)

// SideBid and SideAsk are the byte side codes the tracker keys book levels by.
const (
	SideBid byte = 'b'
	SideAsk byte = 'a'
)

type orderState struct {
	side  byte // 'b' or 'a'
	price float64
	qty   float64
	addTs time.Time
}

type tradePrint struct {
	at     time.Time
	price  float64
	volume float64
}

// l2Level is the per-price aggregate the L2 fallback maintains in place of the
// per-order book L3 provides (§16.4.4). firstSeen is the age proxy.
type l2Level struct {
	qty       float64
	firstSeen time.Time
}

type l2Key struct {
	side  byte
	price float64
}

type levelChurnWindow struct {
	addVol    float64
	deleteVol float64
	started   time.Time
}

type symbolState struct {
	pair       market.Pair
	orders     map[string]*orderState // order_id -> resting order (L3)
	levels     map[l2Key]*l2Level     // (side, price) -> aggregate (L2 fallback)
	churn      map[l2Key]*levelChurnWindow
	bidTotal   float64 // summed visible bid qty
	askTotal   float64
	toxic      map[float64]time.Time // price -> expiry
	toxicChurn map[float64]float64   // price -> cancel/add ratio at flag time
	trades     []tradePrint
	mid        float64
	cancelBid  float64
	fillBid    float64
	cancelAsk  float64
	fillAsk    float64
}

// Tracker classifies book-liquidity removals into fill vs cancel by joining the
// public trade tape, flags large young near-touch cancels as toxic, and reads a
// directional bias from the cancel-to-fill asymmetry. It is fed per-order by
// the authenticated L3 client (ApplyOrder) or per-level by the public L2 book
// fallback (ApplyBookLevel); both share the same classification core.
type Tracker struct {
	mu      sync.Mutex
	symbols map[string]*symbolState
}

func NewTracker() *Tracker {
	return &Tracker{symbols: make(map[string]*symbolState)}
}

func (tracker *Tracker) stateLocked(symbol string, pair market.Pair) *symbolState {
	state := tracker.symbols[symbol]

	if state == nil {
		state = &symbolState{
			pair:       pair,
			orders:     make(map[string]*orderState),
			levels:     make(map[l2Key]*l2Level),
			churn:      make(map[l2Key]*levelChurnWindow),
			toxic:      make(map[float64]time.Time),
			toxicChurn: make(map[float64]float64),
		}
		tracker.symbols[symbol] = state
	}

	return state
}

func (tracker *Tracker) ObserveTrade(symbol string, pair market.Pair, price, volume float64, at time.Time) {
	if price <= 0 || volume <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.stateLocked(symbol, pair)
	state.trades = append(state.trades, tradePrint{at: at, price: price, volume: volume})

	if len(state.trades) > tradeRingCap {
		state.trades = state.trades[len(state.trades)-tradeRingCap:]
	}
}

func (tracker *Tracker) ObserveMid(symbol string, pair market.Pair, mid float64) {
	if mid <= 0 {
		return
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.stateLocked(symbol, pair).mid = mid
}

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

		state.addDepth(order.side, -order.qty)
		tracker.observeLevelChurnLocked(state, order.side, order.price, 0, order.qty, now)
		tracker.classifyRemovalLocked(state, order.side, order.price, order.qty, order.addTs, now)
		delete(state.orders, orderID)

	case "amend":
		order := state.orders[orderID]

		if order == nil {
			state.orders[orderID] = &orderState{side: side, price: price, qty: qty, addTs: ts}
			state.addDepth(side, qty)

			return
		}

		// A price change is remove+add; a quantity cut at the same price is a
		// partial removal of the delta, joined to trades like any removal.
		if price != order.price {
			state.addDepth(order.side, -order.qty)
			tracker.observeLevelChurnLocked(state, order.side, order.price, 0, order.qty, now)
			tracker.classifyRemovalLocked(state, order.side, order.price, order.qty, order.addTs, now)
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
