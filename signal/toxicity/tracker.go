package toxicity

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
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
	pair               krakenmarket.Pair
	orders             map[string]*orderState
	levels             map[l2Key]*l2Level
	churn              map[l2Key]*levelChurnWindow
	bidTotal           float64
	askTotal           float64
	toxic              map[int64]time.Time
	toxicChurn         map[int64]float64
	toxicEvidence      map[int64]float64
	trades             []tradePrint
	mid                float64
	lastPrice          float64
	cancelBid          float64
	fillBid            float64
	cancelAsk          float64
	fillAsk            float64
	levelSizeFracs     []float64
	cancelQtys         []float64
	churnRatios        []float64
	fillMatchRatios    []float64
	vacuumRatios       []float64
	spreadPctEMA       float64
	peakVacuumRatio    float64
	tradeIntervals     []float64
	levelLifetimes     []float64
	bookPulseIntervals []float64
	churnDurations     []float64
	priceIncrements    []float64
	lastTradeAt        time.Time
	hasLastTradeAt     bool
	lastBookPulse      time.Time
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

type Tracker struct {
	symbols              sync.Map
	minFillToCancelRatio atomic.Uint64
}

func NewTracker() *Tracker {
	ratio := viper.GetFloat64("signals.min_fill_to_cancel_ratio")
	tracker := &Tracker{}

	if ratio > 0 {
		tracker.minFillToCancelRatio.Store(math.Float64bits(ratio))
	}

	return tracker
}

var defaultTracker atomic.Pointer[Tracker]

func init() {
	defaultTracker.Store(NewTracker())
}

func Default() *Tracker {
	return defaultTracker.Load()
}

func ResetDefault() {
	defaultTracker.Store(NewTracker())
}

func IsToxic(symbol string, price float64, at time.Time) bool {
	tracker := Default()

	if tracker == nil {
		return false
	}

	return tracker.IsToxic(symbol, price, at)
}

func NearTouchToxic(symbol string, at time.Time) bool {
	tracker := Default()

	if tracker == nil {
		return false
	}

	state := tracker.symbolState(symbol)

	if state == nil {
		return false
	}

	return bookQualitySnapshotLocked(state, at).toxicNear
}

func (tracker *Tracker) fillToCancelThreshold() float64 {
	if bits := tracker.minFillToCancelRatio.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}

	ratio := viper.GetFloat64("signals.min_fill_to_cancel_ratio")

	if ratio <= 0 {
		return 0
	}

	tracker.minFillToCancelRatio.Store(math.Float64bits(ratio))

	return ratio
}

func (tracker *Tracker) symbolState(symbol string) *symbolState {
	raw, ok := tracker.symbols.Load(symbol)

	if !ok {
		return nil
	}

	state, stateOK := raw.(*symbolState)

	if !stateOK {
		return nil
	}

	return state
}

func (tracker *Tracker) stateLocked(symbol string, pair krakenmarket.Pair) *symbolState {
	if state := tracker.symbolState(symbol); state != nil {
		return state
	}

	state := &symbolState{
		pair:          pair,
		orders:        make(map[string]*orderState),
		levels:        make(map[l2Key]*l2Level),
		churn:         make(map[l2Key]*levelChurnWindow),
		toxic:         make(map[int64]time.Time),
		toxicChurn:    make(map[int64]float64),
		toxicEvidence: make(map[int64]float64),
	}
	actual, _ := tracker.symbols.LoadOrStore(symbol, state)

	loaded, ok := actual.(*symbolState)

	if ok {
		return loaded
	}

	return state
}

func (tracker *Tracker) ObserveTrade(symbol string, pair krakenmarket.Pair, price, volume float64, at time.Time) {
	if price <= 0 || volume <= 0 {
		return
	}

	state := tracker.stateLocked(symbol, pair)
	state.lastPrice = price
	state.recordTradeInterval(at)
	state.recordPriceObservation(price)
	state.observeSpreadPct(price)
	state.trades = append(state.trades, tradePrint{at: at, price: price, volume: volume})
	state.trimTrades(at)
}

func (tracker *Tracker) ObserveMid(symbol string, pair krakenmarket.Pair, mid float64) {
	if mid <= 0 {
		return
	}

	tracker.stateLocked(symbol, pair).mid = mid
}

func (tracker *Tracker) ObserveLast(symbol string, pair krakenmarket.Pair, last float64) {
	if last <= 0 {
		return
	}

	tracker.stateLocked(symbol, pair).lastPrice = last
}

func (tracker *Tracker) ApplyBookLevel(
	symbol string, pair krakenmarket.Pair, side byte, price, qty float64, now time.Time,
) {
	if price <= 0 {
		return
	}

	state := tracker.stateLocked(symbol, pair)
	state.recordBookPulse(now)
	tracker.applyBookLevelLocked(state, side, price, qty, now)
}

func (tracker *Tracker) ApplyBookFrame(
	symbol string, pair krakenmarket.Pair, book *krakenmarket.BookUpdate, now time.Time,
) {
	if book == nil {
		return
	}

	state := tracker.stateLocked(symbol, pair)
	state.recordBookPulse(now)
	activeKeys := make(map[l2Key]struct{})

	for _, level := range book.Bids {
		key := l2Key{side: SideBid, price: level.Price}
		activeKeys[key] = struct{}{}
		tracker.applyBookLevelLocked(state, SideBid, level.Price, level.Qty, now)
	}

	for _, level := range book.Asks {
		key := l2Key{side: SideAsk, price: level.Price}
		activeKeys[key] = struct{}{}
		tracker.applyBookLevelLocked(state, SideAsk, level.Price, level.Qty, now)
	}

	for key := range state.levels {
		if _, present := activeKeys[key]; present {
			continue
		}

		tracker.applyBookLevelLocked(state, key.side, key.price, 0, now)
	}
}

func (tracker *Tracker) ApplyBookDelta(
	symbol string, pair krakenmarket.Pair, book *krakenmarket.BookUpdate, now time.Time,
) {
	if book == nil {
		return
	}

	state := tracker.stateLocked(symbol, pair)
	state.recordBookPulse(now)

	for _, level := range book.Bids {
		tracker.applyBookLevelLocked(state, SideBid, level.Price, level.Qty, now)
	}

	for _, level := range book.Asks {
		tracker.applyBookLevelLocked(state, SideAsk, level.Price, level.Qty, now)
	}
}

func (tracker *Tracker) applyBookLevelLocked(
	state *symbolState, side byte, price, qty float64, now time.Time,
) {
	key := l2Key{side: side, price: price}
	state.recordPriceObservation(price)
	level := state.levels[key]

	prevQty := 0.0
	firstSeen := now

	if level != nil {
		prevQty = level.qty
		firstSeen = level.firstSeen
	}

	switch {
	case qty <= 0:
		if prevQty > 0 {
			tracker.observeLevelChurnLocked(state, side, price, 0, prevQty, now)
			tracker.classifyRemovalLocked(state, side, price, prevQty, firstSeen, now)
			state.addDepth(side, -prevQty)
		}

		delete(state.levels, key)

	case qty > prevQty:
		added := qty - prevQty
		state.addDepth(side, added)
		tracker.observeLevelChurnLocked(state, side, price, added, 0, now)

		if level == nil {
			state.levels[key] = &l2Level{qty: qty, firstSeen: now}
			state.recordLevelSizeFrac(added, state.sideDepth(side))

			return
		}

		level.qty = qty
		state.recordLevelSizeFrac(added, state.sideDepth(side))

	case qty < prevQty:
		tracker.observeLevelChurnLocked(state, side, price, 0, prevQty-qty, now)
		tracker.classifyRemovalLocked(state, side, price, prevQty-qty, firstSeen, now)
		state.addDepth(side, qty-prevQty)
		level.qty = qty
	}
}

func (tracker *Tracker) ApplyOrder(
	symbol string, pair krakenmarket.Pair, event, orderID string,
	side byte, price, qty float64, ts, now time.Time,
) {
	if orderID == "" {
		return
	}

	state := tracker.stateLocked(symbol, pair)
	state.recordBookPulse(now)

	switch event {
	case "add":
		if _, exists := state.orders[orderID]; exists {
			return
		}

		state.orders[orderID] = &orderState{side: side, price: price, qty: qty, addTs: ts}
		state.addDepth(side, qty)
		state.recordLevelSizeFrac(qty, state.sideDepth(side))
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

func (state *symbolState) addDepth(side byte, delta float64) {
	if side == 'b' {
		state.bidTotal = math.Max(0, state.bidTotal+delta)

		return
	}

	state.askTotal = math.Max(0, state.askTotal+delta)
}

func (state *symbolState) sideDepth(side byte) float64 {
	if side == 'b' {
		return state.bidTotal
	}

	return state.askTotal
}

func (tracker *Tracker) classifyRemovalLocked(
	state *symbolState, side byte, price, qty float64, addTs, now time.Time,
) {
	matchWindow := state.tradeMatchWindow()
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

	fillGate := state.fillCoverageGate()

	if qty > 0 && matched/qty >= fillGate {
		state.recordFillCoverage(matched, qty)
		tracker.addFlowLocked(state, side, qty, 0)

		return
	}

	tracker.addFlowLocked(state, side, 0, qty)
	state.recordLevelLifetime(now.Sub(addTs))
	state.recordCancelQty(qty)

	sideDepth := state.sideDepth(side)
	largeThreshold := state.largeBlockQtyThreshold(sideDepth)
	large := sideDepth > 0 && qty >= largeThreshold
	proximityPct := state.touchProximityPct()
	distancePct := math.Inf(1)

	if state.mid > 0 {
		distancePct = math.Abs(price-state.mid) / state.mid
	}

	near := proximityPct > 0 && distancePct <= proximityPct
	age := now.Sub(addTs)
	maxAge := state.toxicMaxAge()
	young := maxAge > 0 && age <= maxAge

	if large && near && young {
		evidence := toxicCancelEvidence(
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
	flashWindow := state.flashChurnWindow()

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
	churnGate := state.churnRatioGate()

	if ratio < churnGate {
		return
	}

	state.recordChurnRatio(ratio)

	if state.mid <= 0 {
		return
	}

	distancePct := math.Abs(price-state.mid) / state.mid
	proximityPct := state.touchProximityPct()

	if proximityPct <= 0 || distancePct > proximityPct {
		return
	}

	sideDepth := state.sideDepth(side)
	largeThreshold := state.largeBlockQtyThreshold(sideDepth)

	if sideDepth <= 0 || window.addVol < largeThreshold {
		return
	}

	evidence := toxicChurnEvidence(
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
	alpha := state.flowSmoothingAlpha()

	if alpha <= 0 {
		if side == 'b' {
			state.fillBid = fill
			state.cancelBid = cancel

			return
		}

		state.fillAsk = fill
		state.cancelAsk = cancel

		return
	}

	if side == 'b' {
		state.fillBid += alpha * (fill - state.fillBid)
		state.cancelBid += alpha * (cancel - state.cancelBid)

		return
	}

	state.fillAsk += alpha * (fill - state.fillAsk)
	state.cancelAsk += alpha * (cancel - state.cancelAsk)
}

func (tracker *Tracker) flagToxicLocked(
	state *symbolState,
	price float64,
	churnRatio float64,
	evidence float64,
	now time.Time,
) {
	key := priceKey(state, price)
	state.toxic[key] = now.Add(state.toxicCooldown())

	if churnRatio > 0 {
		state.toxicChurn[key] = churnRatio
	}

	if evidence > 0 {
		state.toxicEvidence[key] = evidence
	}
}

func (tracker *Tracker) IsToxic(symbol string, price float64, at time.Time) bool {

	state := tracker.symbolState(symbol)

	if state == nil {
		return false
	}

	expiry, ok := tracker.toxicExpiryLocked(state, price, at)

	if !ok {
		return false
	}

	if at.After(expiry) {
		return false
	}

	return true
}

func (tracker *Tracker) toxicExpiryLocked(
	state *symbolState,
	price float64,
	at time.Time,
) (time.Time, bool) {
	for _, key := range tickNeighborKeys(state, price) {
		expiry, ok := state.toxic[key]

		if !ok {
			continue
		}

		if at.After(expiry) {
			delete(state.toxic, key)
			delete(state.toxicChurn, key)
			delete(state.toxicEvidence, key)

			continue
		}

		return expiry, true
	}

	return time.Time{}, false
}

func (tracker *Tracker) vacuumStrengthLimit(symbol string, threshold, maxRatio float64) float64 {

	state := tracker.symbolState(symbol)

	if state == nil || threshold <= 0 {
		if threshold <= 0 {
			return 1
		}

		return math.Max(2, maxRatio/threshold)
	}

	state.recordVacuumRatio(maxRatio)
	state.peakVacuumRatio = math.Max(state.peakVacuumRatio, maxRatio)

	return state.vacuumStrengthLimit(threshold)
}

func (tracker *Tracker) churnRatioGate(symbol string) float64 {

	state := tracker.symbolState(symbol)

	if state == nil {
		return 1
	}

	return state.churnRatioGate()
}

func (tracker *Tracker) supportRatioGate(symbol string, threshold float64) float64 {

	state := tracker.symbolState(symbol)

	if state == nil {
		return 0
	}

	return state.supportRatioGate(threshold)
}

func (tracker *Tracker) Snapshot(symbol string, at time.Time) (bookQualitySnapshot, float64, bool) {

	state := tracker.symbolState(symbol)

	if state == nil {
		return bookQualitySnapshot{}, 0, false
	}

	return bookQualitySnapshotLocked(state, at), state.lastPrice, true
}

func bookQualitySnapshotLocked(state *symbolState, at time.Time) bookQualitySnapshot {
	snapshot := bookQualitySnapshot{
		cancelBid: state.cancelBid,
		fillBid:   state.fillBid,
		cancelAsk: state.cancelAsk,
		fillAsk:   state.fillAsk,
		bidDepth:  state.bidTotal,
		askDepth:  state.askTotal,
	}

	for key, expiry := range state.toxic {
		if at.After(expiry) {
			delete(state.toxic, key)
			delete(state.toxicChurn, key)
			delete(state.toxicEvidence, key)

			continue
		}

		price := priceFromKey(state, key)

		proximityPct := state.touchProximityPct()

		if state.mid > 0 && proximityPct > 0 &&
			math.Abs(price-state.mid)/state.mid <= proximityPct {
			snapshot.toxicNear = true
			snapshot.toxicBluffStrength = math.Max(
				snapshot.toxicBluffStrength,
				math.Max(state.toxicChurn[key], state.toxicEvidence[key]),
			)
		}
	}

	return snapshot
}

func toxicCancelEvidence(
	qty float64,
	sizeThreshold float64,
	distancePct float64,
	proximityPct float64,
	age time.Duration,
	maxAge time.Duration,
) float64 {
	if sizeThreshold <= 0 || qty < sizeThreshold {
		return 0
	}

	if proximityPct <= 0 || distancePct > proximityPct || maxAge <= 0 || age > maxAge {
		return 0
	}

	sizeEvidence := competitionMargin(qty-sizeThreshold, sizeThreshold)
	proximityEvidence := competitionMargin(proximityPct-distancePct, proximityPct)
	ageEvidence := competitionMargin(float64(maxAge-age), float64(maxAge))

	return evidenceGeomean(sizeEvidence, proximityEvidence, ageEvidence)
}

func toxicChurnEvidence(
	ratio float64,
	churnGate float64,
	addVol float64,
	sizeThreshold float64,
	distancePct float64,
	proximityPct float64,
) float64 {
	if ratio <= churnGate || sizeThreshold <= 0 || addVol < sizeThreshold {
		return 0
	}

	if proximityPct <= 0 || distancePct > proximityPct {
		return 0
	}

	ratioEvidence := competitionMargin(ratio-churnGate, churnGate)
	sizeEvidence := competitionMargin(addVol-sizeThreshold, sizeThreshold)
	proximityEvidence := competitionMargin(proximityPct-distancePct, proximityPct)

	return evidenceGeomean(ratioEvidence, sizeEvidence, proximityEvidence)
}

func evidenceGeomean(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}

	product := 1.0

	for _, value := range values {
		if value <= 0 {
			return 0
		}

		product *= value
	}

	return math.Pow(product, 1/float64(len(values)))
}

func competitionMargin(excess, span float64) float64 {
	if excess <= 0 || span <= 0 {
		return 0
	}

	return excess / (excess + span)
}

func magnitudeMargin(value float64) float64 {
	if value <= 0 {
		return 0
	}

	return value / (1 + value)
}

func cancelFillRatio(cancel, fill float64) float64 {
	if cancel <= 0 || fill <= 0 {
		return 0
	}

	return cancel / fill
}

func tickNeighborKeys(state *symbolState, price float64) []int64 {
	center := priceKey(state, price)

	return []int64{center - 1, center, center + 1}
}

func priceKey(state *symbolState, price float64) int64 {
	tickSize, err := state.pair.TickSizeFloat()

	if err == nil && tickSize > 0 {
		rounded := math.Round(price / tickSize)

		if rounded > float64(math.MaxInt64) {
			return math.MaxInt64
		}

		if rounded < float64(math.MinInt64) {
			return math.MinInt64
		}

		return int64(rounded)
	}

	scale := state.priceKeyScale()

	if scale <= 0 {
		return 0
	}

	rounded := math.Round(price * scale)

	if rounded > float64(math.MaxInt64) {
		return math.MaxInt64
	}

	if rounded < float64(math.MinInt64) {
		return math.MinInt64
	}

	return int64(rounded)
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
