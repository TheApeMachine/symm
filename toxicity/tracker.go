package toxicity

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

const (
	tradeMatchWindow  = 2 * time.Second
	priceMatchTol     = 0.0002 // tight: L3 gives exact prices
	fillCoverage      = 0.5    // matched trade vol >= this x removed qty => fill
	toxicMaxAge       = 10 * time.Second
	toxicProximityPct = 0.005 // 0.5 % of mid
	largeBlockFrac    = 0.10  // >= 10 % of that side's visible depth = "large"
	toxicCooldown     = 30 * time.Second
	flowAlpha         = 0.05
	tradeRingCap      = 512
	epsilon           = 1e-12
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

type symbolState struct {
	pair      asset.Pair
	orders    map[string]*orderState // order_id -> resting order (L3)
	levels    map[l2Key]*l2Level     // (side, price) -> aggregate (L2 fallback)
	bidTotal  float64                // summed visible bid qty
	askTotal  float64
	toxic     map[float64]time.Time // price -> expiry
	trades    []tradePrint
	mid       float64
	cancelBid float64
	fillBid   float64
	cancelAsk float64
	fillAsk   float64
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

func (tracker *Tracker) stateLocked(symbol string, pair asset.Pair) *symbolState {
	state := tracker.symbols[symbol]

	if state == nil {
		state = &symbolState{
			pair:   pair,
			orders: make(map[string]*orderState),
			levels: make(map[l2Key]*l2Level),
			toxic:  make(map[float64]time.Time),
		}
		tracker.symbols[symbol] = state
	}

	return state
}

func (tracker *Tracker) ObserveTrade(symbol string, pair asset.Pair, price, volume float64, at time.Time) {
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

func (tracker *Tracker) ObserveMid(symbol string, pair asset.Pair, mid float64) {
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
	symbol string, pair asset.Pair, event, orderID string,
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

	case "delete":
		order := state.orders[orderID]
		if order == nil {
			return
		}

		state.addDepth(order.side, -order.qty)
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
			tracker.classifyRemovalLocked(state, order.side, order.price, order.qty, order.addTs, now)
			order.side, order.price, order.qty, order.addTs = side, price, qty, ts
			state.addDepth(side, qty)

			return
		}

		if delta := qty - order.qty; delta < 0 {
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
	symbol string, pair asset.Pair, side byte, price, qty float64, now time.Time,
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

		if level == nil {
			state.levels[key] = &l2Level{qty: qty, firstSeen: now}

			return
		}

		level.qty = qty

	case qty < prevQty:
		// Level shrank: classify the removed delta.
		state.addDepth(side, qty-prevQty)
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
		state.toxic[price] = now.Add(toxicCooldown)
	}
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

		return false
	}

	return true
}

// Measure emits a directional read from the cancel-to-fill asymmetry: a side
// whose resting liquidity is pulled (high cancel-to-fill) while the other side
// keeps executing signals intent to move price away from the pulled side.
func (tracker *Tracker) Measure(symbol string, now time.Time) (engine.Measurement, bool) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.symbols[symbol]
	if state == nil {
		return engine.Measurement{}, false
	}

	askPull := state.cancelAsk / (state.fillAsk + epsilon)
	bidPull := state.cancelBid / (state.fillBid + epsilon)

	if askPull > bidPull && state.fillBid > 0 {
		return state.emit(engine.Momentum, "bookflow", "ask_pull", state.pullCategory(now), squash(askPull-bidPull), now), true
	}

	if bidPull > askPull && state.fillAsk > 0 {
		return state.emit(engine.Dump, "bookflow", "bid_pull", state.pullCategory(now), squash(bidPull-askPull), now), true
	}

	// Neither side is retreating and fills dominate cancels on both sides (pull
	// ratio below 1): the book is sincere -- hard support. Confidence rises as
	// cancels fall further below fills.
	if (state.fillBid > 0 || state.fillAsk > 0) && askPull < 1 && bidPull < 1 {
		return state.emit(engine.Momentum, "bookflow", "hard_support", engine.CatHardSupport,
			squash(1-(askPull+bidPull)/2), now), true
	}

	return engine.Measurement{}, false
}

/*
pullCategory distinguishes a manipulative near-touch bluff (a large, young
order being cancelled rather than filled, the signature of fake support) from
an honest liquidity vacuum (one side simply retreating). The toxic-bluff read
is what the decision layer treats as a hard manipulation veto.
*/
func (state *symbolState) pullCategory(now time.Time) engine.Category {
	if state.hasToxicLevel(now) {
		return engine.CatToxicBluff
	}

	return engine.CatLiquidityVacuum
}

// hasToxicLevel reports whether any large/young/near-touch cancel is still
// within its toxic cooldown, pruning expired entries as it scans.
func (state *symbolState) hasToxicLevel(now time.Time) bool {
	active := false

	for price, expiry := range state.toxic {
		if now.After(expiry) {
			delete(state.toxic, price)
			continue
		}

		active = true
	}

	return active
}

func (state *symbolState) emit(
	mtype engine.MeasurementType, regime, reason string, category engine.Category, confidence float64, now time.Time,
) engine.Measurement {
	return engine.Measurement{
		Type:       mtype,
		Source:     "bookflow",
		Regime:     regime,
		Reason:     reason,
		Category:   category,
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.mid,
	}
}

func squash(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value / (1 + value)
}
