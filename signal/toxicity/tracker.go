package toxicity

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/statistic"
)

const (
	SideBid byte = 'b'
	SideAsk byte = 'a'
)

type orderState struct {
	side  byte
	price float64
	qty   float64
	addTs time.Time
}

type tradePrint struct {
	at     time.Time
	price  float64
	volume float64
}

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
	pair            Pair
	timing          *adaptive.TimedContext
	gates           *algorithm.BookGates
	flow            algorithm.SideFlowLedger
	toxic           *algorithm.EvidenceRegistry
	priceIncrements *statistic.ObservationRing
	orders          map[string]*orderState
	levels          map[l2Key]*l2Level
	churn           map[l2Key]*levelChurnWindow
	trades          []tradePrint
	mid             float64
	lastPrice       float64
	spreadPctEMA    float64
	peakVacuumRatio float64
	lastTradeAt     time.Time
	hasLastTradeAt  bool
	lastBookPulse   time.Time
	lastEventAt     time.Time
}

type bookQualitySnapshot struct {
	cancelBid          float64
	fillBid            float64
	cancelAsk          float64
	fillAsk            float64
	bidDepth           float64
	askDepth           float64
	toxicNear          bool
	toxicBluffStrength float64
}

type measureFeaturesReply struct {
	snapshot          bookQualitySnapshot
	lastPrice         float64
	threshold         float64
	churnGate         float64
	supportGate       float64
	vacuumStrengthCap float64
}

type snapshotResult struct {
	snapshot  bookQualitySnapshot
	lastPrice float64
	ok        bool
}

type Tracker struct {
	symbols              sync.Map
	minFillToCancelRatio atomic.Uint64
	observations         *dmt.Tree
}

func NewTracker() *Tracker {
	return &Tracker{
		observations: newObservationTree(),
	}
}

func NewConcurrentTracker(ctx context.Context) *Tracker {
	if ctx == nil {
		ctx = context.Background()
	}

	_ = ctx

	return NewTracker()
}

func (tracker *Tracker) Close() {
	if tracker == nil || tracker.observations == nil {
		return
	}

	errnie.Error(tracker.observations.Close())
}

func readTrackerState[T any](
	tracker *Tracker,
	symbol string,
	zero T,
	work func(*symbolState) T,
) T {
	if tracker == nil || work == nil {
		return zero
	}

	raw, ok := tracker.symbols.Load(symbol)

	if !ok {
		return zero
	}

	slot, slotOK := raw.(*symbolSlot)

	if !slotOK {
		return zero
	}

	state := slot.state.Load()

	if state == nil {
		return zero
	}

	return work(state)
}

func (tracker *Tracker) withState(
	symbol string,
	pair Pair,
	at time.Time,
	bookPulse bool,
	role string,
	work func(*symbolState),
) {
	tracker.mutateState(symbol, pair, at, bookPulse, role, work)
}

var defaultTracker atomic.Pointer[Tracker]

func init() {
	defaultTracker.Store(NewConcurrentTracker(context.Background()))
}

func ResetDefault() {
	defaultTracker.Store(NewTracker())
}

func IsToxic(symbol string, price float64, at time.Time) bool {
	return defaultTracker.Load().IsToxic(symbol, price, at)
}

func NearTouchToxic(symbol string, at time.Time) bool {
	return defaultTracker.Load().nearTouchToxic(symbol, at)
}

func (tracker *Tracker) nearTouchToxic(symbol string, at time.Time) bool {
	return readTrackerState(tracker, symbol, false, func(state *symbolState) bool {
		return bookQualitySnapshotLocked(state, at).toxicNear
	})
}

func (tracker *Tracker) fillToCancelThreshold() float64 {
	return tracker.fillToCancelThresholdLocked()
}

func (tracker *Tracker) fillToCancelThresholdLocked() float64 {
	if bits := tracker.minFillToCancelRatio.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}

	ratio := tracker.derivedFillToCancelMedianLocked()

	if ratio <= 0 {
		return 0
	}

	tracker.minFillToCancelRatio.Store(math.Float64bits(ratio))

	return ratio
}

func (tracker *Tracker) derivedFillToCancelMedianLocked() float64 {
	ratios := make([]float64, 0, 8)

	tracker.symbols.Range(func(_, raw any) bool {
		slot, ok := raw.(*symbolSlot)

		if !ok || slot == nil {
			return true
		}

		state := slot.state.Load()

		if state == nil {
			return true
		}

		denominator := state.flow.CancelBid + state.flow.CancelAsk + state.flow.FillBid + state.flow.FillAsk

		if denominator <= 0 {
			return true
		}

		ratio := (state.flow.FillBid + state.flow.FillAsk) / denominator

		if ratio > 0 {
			ratios = append(ratios, ratio)
		}

		return true
	})

	if len(ratios) == 0 {
		return 0
	}

	return statistic.MedianOf(ratios)
}

func (tracker *Tracker) measureFeatures(symbol string) (measureFeaturesReply, bool) {
	result := readTrackerState(tracker, symbol, struct {
		measureFeaturesReply
		ok bool
	}{}, func(state *symbolState) struct {
		measureFeaturesReply
		ok bool
	} {
		if state.lastPrice <= 0 {
			return struct {
				measureFeaturesReply
				ok bool
			}{}
		}

		eventAt := time.Now()

		if !state.lastEventAt.IsZero() {
			eventAt = state.lastEventAt
		}

		snapshot := bookQualitySnapshotLocked(state, eventAt)
		threshold := tracker.fillToCancelThresholdLocked()
		churnGate := state.gates.ChurnRatioGate()
		supportGate := state.gates.SupportRatioGate(threshold)
		bidRatio := algorithm.CancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
		askRatio := algorithm.CancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
		maxRatio := math.Max(bidRatio, askRatio)
		vacuumStrengthCap := tracker.vacuumStrengthLimitLocked(state, threshold, maxRatio)

		return struct {
			measureFeaturesReply
			ok bool
		}{
			measureFeaturesReply: measureFeaturesReply{
				snapshot:          snapshot,
				lastPrice:         state.lastPrice,
				threshold:         threshold,
				churnGate:         churnGate,
				supportGate:       supportGate,
				vacuumStrengthCap: vacuumStrengthCap,
			},
			ok: true,
		}
	})

	return result.measureFeaturesReply, result.ok
}

func (tracker *Tracker) symbolState(symbol string) *symbolState {
	raw, ok := tracker.symbols.Load(symbol)

	if !ok {
		return nil
	}

	slot, slotOK := raw.(*symbolSlot)

	if !slotOK {
		return nil
	}

	return slot.state.Load()
}

func (tracker *Tracker) stateLocked(symbol string, pair Pair) *symbolState {
	slot := tracker.loadSlot(symbol, pair)
	state := slot.state.Load()

	if state != nil {
		return state
	}

	fresh := newSymbolState(pair)
	slot.state.Store(fresh)

	return fresh
}

func (tracker *Tracker) ObserveTrade(
	symbol string,
	pair Pair,
	price, volume float64,
	at time.Time,
) {
	if price <= 0 || volume <= 0 {
		return
	}

	tracker.withState(symbol, pair, at, false, "trade", func(state *symbolState) {
		state.lastPrice = price
		state.recordTradeInterval(at)
		state.recordPriceObservation(price)
		state.observeSpreadPct(price)
		state.trades = append(state.trades, tradePrint{at: at, price: price, volume: volume})
		state.trimTrades(at)
	})
}

func (tracker *Tracker) ObserveMid(symbol string, pair Pair, mid float64) {
	if mid <= 0 {
		return
	}

	tracker.withState(symbol, pair, time.Now(), false, "mid", func(state *symbolState) {
		state.mid = mid
	})
}

func (tracker *Tracker) ObserveLast(symbol string, pair Pair, last float64) {
	if last <= 0 {
		return
	}

	tracker.withState(symbol, pair, time.Now(), false, "last", func(state *symbolState) {
		state.lastPrice = last
	})
}

func (tracker *Tracker) ApplyBookLevel(
	symbol string,
	pair Pair,
	side byte,
	price, qty float64,
	now time.Time,
) {
	if price <= 0 {
		return
	}

	tracker.withState(symbol, pair, now, true, "book", func(state *symbolState) {
		tracker.applyBookLevelLocked(state, side, price, qty, now)
	})
}

func eachBookLevel(book *BookUpdate, work func(byte, float64, float64)) {
	for _, level := range book.Bids {
		work(SideBid, level.Price, level.Qty)
	}

	for _, level := range book.Asks {
		work(SideAsk, level.Price, level.Qty)
	}
}

func (tracker *Tracker) ApplyBookFrame(
	symbol string,
	pair Pair,
	book *BookUpdate,
	now time.Time,
) {
	if book == nil {
		return
	}

	tracker.withState(symbol, pair, now, true, "book", func(state *symbolState) {
		active := make(map[l2Key]struct{}, len(book.Bids)+len(book.Asks))

		eachBookLevel(book, func(side byte, price, qty float64) {
			key := l2Key{side: side, price: price}
			active[key] = struct{}{}
			tracker.applyBookLevelLocked(state, side, price, qty, now)
		})

		for key := range state.levels {
			if _, ok := active[key]; ok {
				continue
			}

			tracker.applyBookLevelLocked(state, key.side, key.price, 0, now)
		}
	})
}

func (tracker *Tracker) ApplyBookDelta(
	symbol string,
	pair Pair,
	book *BookUpdate,
	now time.Time,
) {
	if book == nil {
		return
	}

	tracker.withState(symbol, pair, now, true, "book", func(state *symbolState) {
		eachBookLevel(book, func(side byte, price, qty float64) {
			tracker.applyBookLevelLocked(state, side, price, qty, now)
		})
	})
}

func levelAt(state *symbolState, key l2Key, now time.Time) (*l2Level, float64, time.Time) {
	level := state.levels[key]

	if level == nil {
		return nil, 0, now
	}

	return level, level.qty, level.firstSeen
}

func (tracker *Tracker) addDepthLocked(
	state *symbolState,
	side byte,
	price, qty float64,
	now time.Time,
) {
	if qty <= 0 {
		return
	}

	state.flow.AddDepth(side, qty)
	tracker.observeLevelChurnLocked(state, side, price, qty, 0, now)
	observeLevelSizeFraction(state, side, qty)
}

func (tracker *Tracker) removeDepthLocked(
	state *symbolState,
	side byte,
	price, qty float64,
	firstSeen, now time.Time,
) {
	if qty <= 0 {
		return
	}

	tracker.observeLevelChurnLocked(state, side, price, 0, qty, now)
	tracker.classifyRemovalLocked(state, side, price, qty, firstSeen, now)
	state.flow.AddDepth(side, -qty)
}

func (tracker *Tracker) applyBookLevelLocked(
	state *symbolState,
	side byte,
	price, qty float64,
	now time.Time,
) {
	key := l2Key{side: side, price: price}
	state.recordPriceObservation(price)

	level, prevQty, firstSeen := levelAt(state, key, now)

	switch delta := qty - prevQty; {
	case qty <= 0:
		tracker.removeDepthLocked(state, side, price, prevQty, firstSeen, now)
		delete(state.levels, key)

	case delta > 0:
		tracker.addDepthLocked(state, side, price, delta, now)

		if level == nil {
			state.levels[key] = &l2Level{qty: qty, firstSeen: now}

			return
		}

		level.qty = qty

	case delta < 0:
		tracker.removeDepthLocked(state, side, price, -delta, firstSeen, now)
		level.qty = qty
	}
}

func (tracker *Tracker) ApplyOrder(
	symbol string,
	pair Pair,
	event, orderID string,
	side byte,
	price, qty float64,
	ts, now time.Time,
) {
	if orderID == "" {
		return
	}

	next := orderState{side: side, price: price, qty: qty, addTs: ts}

	tracker.withState(symbol, pair, now, true, "book", func(state *symbolState) {
		switch event {
		case "add":
			tracker.addOrderLocked(state, orderID, next, now)

		case "delete":
			tracker.deleteOrderLocked(state, orderID, now)

		case "modify", "amend":
			tracker.modifyOrderLocked(state, orderID, next, now)
		}
	})
}

func (tracker *Tracker) addOrderLocked(
	state *symbolState,
	orderID string,
	order orderState,
	now time.Time,
) {
	if _, exists := state.orders[orderID]; exists {
		return
	}

	state.orders[orderID] = &order
	tracker.addDepthLocked(state, order.side, order.price, order.qty, now)
}

func (tracker *Tracker) deleteOrderLocked(
	state *symbolState,
	orderID string,
	now time.Time,
) {
	order := state.orders[orderID]

	if order == nil {
		return
	}

	tracker.removeDepthLocked(state, order.side, order.price, order.qty, order.addTs, now)
	delete(state.orders, orderID)
}

func (tracker *Tracker) modifyOrderLocked(
	state *symbolState,
	orderID string,
	next orderState,
	now time.Time,
) {
	order := state.orders[orderID]

	if order == nil {
		state.orders[orderID] = &next
		state.flow.AddDepth(next.side, next.qty)

		return
	}

	if order.side != next.side || order.price != next.price {
		tracker.deleteOrderLocked(state, orderID, now)
		tracker.addOrderLocked(state, orderID, next, now)

		return
	}

	if delta := next.qty - order.qty; delta > 0 {
		tracker.addDepthLocked(state, order.side, order.price, delta, now)
	}

	if delta := next.qty - order.qty; delta < 0 {
		tracker.removeDepthLocked(state, order.side, order.price, -delta, order.addTs, now)
	}

	order.qty = next.qty
}

func (tracker *Tracker) classifyRemovalLocked(
	state *symbolState, side byte, price, qty float64, addTs, now time.Time,
) {
	matchWindow := state.timing.MatchWindow(state.tradeSpan())
	matched := 0.0
	cutoff := now.Add(-matchWindow)

	for _, trade := range state.trades {
		if trade.at.Before(cutoff) {
			continue
		}

		if math.Abs(trade.price-price)/price <= state.priceMatchTolerance(price) {
			matched += trade.volume
		}
	}

	fillGate := state.gates.FillCoverageGate()

	if qty > 0 && matched/qty >= fillGate {
		state.gates.FillMatchRatios.Observe(matched / qty)
		tracker.addFlowLocked(state, side, qty, 0)

		return
	}

	tracker.addFlowLocked(state, side, 0, qty)
	state.recordLevelLifetime(now.Sub(addTs))

	if qty > 0 {
		state.gates.CancelQtys.Observe(qty)
	}

	sideDepth := state.flow.SideDepth(side)
	largeThreshold := state.gates.LargeBlockQtyThreshold(sideDepth, medianLevelQty(state.levels))
	large := sideDepth > 0 && qty >= largeThreshold
	proximityPct := state.touchProximityPct()
	distancePct := math.Inf(1)

	if state.mid > 0 {
		distancePct = math.Abs(price-state.mid) / state.mid
	}

	near := proximityPct > 0 && distancePct <= proximityPct
	age := now.Sub(addTs)
	maxAge := state.timing.MaxAge()
	young := maxAge > 0 && age <= maxAge

	if large && near && young {
		evidence := algorithm.ToxicCancelEvidence(
			qty,
			largeThreshold,
			distancePct,
			proximityPct,
			age,
			maxAge,
		)

		tracker.flagToxicLocked(state, price, 0, evidence, now)
	}
}

func (tracker *Tracker) observeLevelChurnLocked(
	state *symbolState, side byte, price, addVol, deleteVol float64, now time.Time,
) {
	if price <= 0 || (addVol <= 0 && deleteVol <= 0) {
		return
	}

	key := l2Key{side: side, price: price}
	window := state.churn[key]
	flashWindow := state.timing.FlashWindow()

	if window != nil && flashWindow > 0 && now.Sub(window.started) > flashWindow {
		state.recordChurnDuration(now.Sub(window.started))
		window = nil
	}

	if window == nil {
		window = &levelChurnWindow{started: now}
		state.churn[key] = window
	}

	window.addVol += addVol
	window.deleteVol += deleteVol

	if window.addVol <= 0 {
		return
	}

	ratio := window.deleteVol / window.addVol
	churnGate := state.gates.ChurnRatioGate()

	if ratio < churnGate {
		return
	}

	state.gates.ChurnRatios.Observe(ratio)

	if state.mid <= 0 {
		return
	}

	distancePct := math.Abs(price-state.mid) / state.mid
	proximityPct := state.touchProximityPct()

	if proximityPct <= 0 || distancePct > proximityPct {
		return
	}

	sideDepth := state.flow.SideDepth(side)
	largeThreshold := state.gates.LargeBlockQtyThreshold(sideDepth, medianLevelQty(state.levels))

	if sideDepth <= 0 || window.addVol < largeThreshold {
		return
	}

	evidence := algorithm.ToxicChurnEvidence(
		ratio,
		churnGate,
		window.addVol,
		largeThreshold,
		distancePct,
		proximityPct,
	)

	tracker.flagToxicLocked(state, price, ratio, evidence, now)
	state.recordChurnDuration(now.Sub(window.started))
}

func (tracker *Tracker) addFlowLocked(
	state *symbolState,
	side byte,
	fill, cancel float64,
) {
	state.flow.ApplyFlow(
		side,
		fill,
		cancel,
		state.timing.FlowSmoothingAlpha(
			state.timing.MatchWindow(state.tradeSpan()),
			state.tradeSpan(),
			len(state.trades),
		),
	)
}

func (tracker *Tracker) flagToxicLocked(
	state *symbolState,
	price float64,
	churnRatio float64,
	evidence float64,
	now time.Time,
) {
	cooldown := state.timing.Cooldown(state.timing.MatchWindow(state.tradeSpan()))
	expires := now.Add(cooldown)

	if cooldown <= 0 {
		expires = now
	}

	state.toxic.Flag(
		priceKey(state, price),
		churnRatio,
		evidence,
		expires,
	)
}

func (tracker *Tracker) IsToxic(symbol string, price float64, at time.Time) bool {
	return readTrackerState(tracker, symbol, false, func(state *symbolState) bool {
		expiry, _, _, ok := state.toxic.ActiveExpiry(tickNeighborKeys(state, price), at)

		return ok && !at.After(expiry)
	})
}

func (tracker *Tracker) vacuumStrengthLimit(symbol string, threshold, maxRatio float64) float64 {
	if threshold <= 0 {
		return 1
	}

	fallback := math.Max(2, maxRatio/threshold)

	return readTrackerState(tracker, symbol, fallback, func(state *symbolState) float64 {
		return tracker.vacuumStrengthLimitLocked(state, threshold, maxRatio)
	})
}

func (tracker *Tracker) vacuumStrengthLimitLocked(
	state *symbolState,
	threshold, maxRatio float64,
) float64 {
	state.gates.VacuumRatios.Observe(maxRatio)
	state.peakVacuumRatio = math.Max(state.peakVacuumRatio, maxRatio)

	return state.gates.VacuumStrengthLimit(threshold, state.peakVacuumRatio)
}

func (tracker *Tracker) Snapshot(symbol string, at time.Time) (bookQualitySnapshot, float64, bool) {
	result := readTrackerState(tracker, symbol, snapshotResult{}, func(state *symbolState) snapshotResult {
		return snapshotResult{
			snapshot:  bookQualitySnapshotLocked(state, at),
			lastPrice: state.lastPrice,
			ok:        true,
		}
	})

	return result.snapshot, result.lastPrice, result.ok
}

func bookQualitySnapshotLocked(state *symbolState, at time.Time) bookQualitySnapshot {
	snapshot := bookQualitySnapshot{}
	snapshot.cancelBid, snapshot.fillBid, snapshot.cancelAsk, snapshot.fillAsk, snapshot.bidDepth, snapshot.askDepth = state.flow.Snapshot()

	near, strength := state.toxic.NearTouchStrength(
		state.mid,
		state.touchProximityPct(),
		at,
		func(key int64) float64 {
			return priceFromKey(state, key)
		},
	)
	snapshot.toxicNear = near
	snapshot.toxicBluffStrength = strength

	return snapshot
}

func tickNeighborKeys(state *symbolState, price float64) []int64 {
	center := priceKey(state, price)
	return []int64{center - 1, center, center + 1}
}

func priceKey(state *symbolState, price float64) int64 {
	if tickSize, err := state.pair.TickSizeFloat(); err == nil && tickSize > 0 {
		return clampRoundedInt64(price / tickSize)
	}

	if scale := state.priceKeyScale(); scale > 0 {
		return clampRoundedInt64(price * scale)
	}

	return 0
}

func priceFromKey(state *symbolState, key int64) float64 {
	tickSize, err := state.pair.TickSizeFloat()

	if err == nil && tickSize > 0 {
		return float64(key) * tickSize
	}

	scale := state.priceKeyScale()

	if scale <= 0 {
		return 0
	}

	return float64(key) / scale
}
