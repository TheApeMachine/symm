package tables

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
SymbolTracker maintains order book state, precursor buffers, and active excursion
tracking for one instrument.
*/
type symbolTracker struct {
	symbol         string
	bid            float64
	ask            float64
	last           float64
	spread         float64
	precursorTicks []int64
	quietTicks     int
	lastFlatTick   int64

	// Active excursion tracking
	inExcursion        bool
	direction          string
	anchorTick         int64
	anchorAt           time.Time
	anchorPrice        float64
	precursorStartTick int64
	extremumTick       int64
	extremumAt         time.Time
	extremumPrice      float64
	entryPrice         float64
	positionSize       float64
	entryFee           float64
	observationCount   int64

	// Exit and aftermath
	exited             bool
	exitTick           int64
	exitAt             time.Time
	exitPrice          float64
	exitStatus         string
	postTicksCollected int
	stagnationCounter  int
}

func newSymbolTracker(symbol string) *symbolTracker {
	return &symbolTracker{
		symbol:         symbol,
		precursorTicks: make([]int64, 0, 32),
	}
}

/*
StreamingDetector monitors incoming data.Measurement streams from the workspace Tee,
detects real executable opportunities dealing with live order book and trade prices,
maintains multi-symbol balance allocation (20% of available cash per opportunity),
and yields ExcursionRecords when moves exhaust or reverse.

It balances training data across:
1. Profitable upward excursions that clear friction (spread + taker fees)
2. Sub-friction upward excursions where fees exceed move
3. Downward excursions (negative training tape for entry inhibition)
4. Choppy/flat tape fragments where entry bleeds spread and fees
*/
type StreamingDetector struct {
	mutex            sync.Mutex
	epoch            int64
	availableBalance float64
	feeRate          float64
	precursorWindow  int
	postMarginWindow int
	trackers         map[string]*symbolTracker
	onComplete       func(ExcursionRecord)

	// Balanced training sample quotas
	profitableCount  int64
	subfrictionCount int64
	downwardCount    int64
	flatCount        int64
}

/*
NewStreamingDetector constructs a StreamingDetector for an epoch and initial cash balance.
*/
func NewStreamingDetector(
	epoch int64,
	initialBalance float64,
	onComplete func(ExcursionRecord),
) *StreamingDetector {
	balance := initialBalance

	if balance <= 0 {
		balance = 200.0 // Canonical initial paper/test balance
	}

	return &StreamingDetector{
		epoch:            epoch,
		availableBalance: balance,
		feeRate:          0.0026, // Authoritative Kraken taker fee rate
		precursorWindow:  16,
		postMarginWindow: 8,
		trackers:         make(map[string]*symbolTracker),
		onComplete:       onComplete,
	}
}

/*
Process inspects one incoming measurement in streaming fashion.
*/
func (detector *StreamingDetector) Process(measurement *data.Measurement[float64]) {
	if measurement == nil || measurement.Label == "" {
		return
	}

	detector.mutex.Lock()
	defer detector.mutex.Unlock()

	symbol := measurement.Label
	tracker, exists := detector.trackers[symbol]

	if !exists {
		tracker = newSymbolTracker(symbol)
		detector.trackers[symbol] = tracker
	}

	detector.updatePrices(tracker, measurement)

	if tracker.bid <= 0 && tracker.ask <= 0 && tracker.last <= 0 {
		return
	}

	if tracker.inExcursion {
		detector.advanceExcursion(tracker, measurement)

		return
	}

	detector.recordPrecursor(tracker, measurement.SeqIdx)
	detector.checkForExcursionStart(tracker, measurement)
}

func (detector *StreamingDetector) updatePrices(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	if measurement.Source == "spot_ticker" || measurement.Source == "ticker" {
		if bidMetric, ok := measurement.Metrics["bid"]; ok && bidMetric.Raw > 0 {
			tracker.bid = bidMetric.Raw
		}

		if askMetric, ok := measurement.Metrics["ask"]; ok && askMetric.Raw > 0 {
			tracker.ask = askMetric.Raw
		}

		if lastMetric, ok := measurement.Metrics["last"]; ok && lastMetric.Raw > 0 {
			tracker.last = lastMetric.Raw
		}

		if tracker.bid > 0 && tracker.ask > 0 && tracker.ask >= tracker.bid {
			tracker.spread = tracker.ask - tracker.bid
		}

		return
	}

	if measurement.Source == "spot_trade" || measurement.Source == "trade" {
		if priceMetric, ok := measurement.Metrics["price"]; ok && priceMetric.Raw > 0 {
			tracker.last = priceMetric.Raw
		}

		return
	}

	if tracker.last <= 0 && tracker.bid > 0 {
		tracker.last = tracker.bid
	}
}

func (detector *StreamingDetector) recordPrecursor(tracker *symbolTracker, tick int64) {
	tracker.precursorTicks = append(tracker.precursorTicks, tick)

	if len(tracker.precursorTicks) > detector.precursorWindow {
		tracker.precursorTicks = tracker.precursorTicks[1:]
	}
}

func (detector *StreamingDetector) checkForExcursionStart(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	ask := tracker.ask

	if ask <= 0 {
		ask = tracker.last
	}

	bid := tracker.bid

	if bid <= 0 {
		bid = tracker.last
	}

	if ask <= 0 || bid <= 0 {
		return
	}

	if tracker.anchorPrice <= 0 {
		tracker.anchorTick = measurement.SeqIdx
		tracker.anchorAt = time.Now().UTC()
		tracker.anchorPrice = (bid + ask) / 2

		return
	}

	// Trigger initiation at early departure (0.15% or 0.5 * fee) so we also
	// observe sub-friction moves that start but fail before clearing friction.
	initiationMove := 0.0015
	frictionRate := 2 * detector.feeRate

	if tracker.spread > 0 && tracker.anchorPrice > 0 {
		frictionRate += tracker.spread / tracker.anchorPrice
	}

	if frictionRate*0.5 > initiationMove {
		initiationMove = frictionRate * 0.5
	}

	upwardMove := (ask - tracker.anchorPrice) / tracker.anchorPrice
	downwardMove := (tracker.anchorPrice - bid) / tracker.anchorPrice

	if upwardMove >= initiationMove {
		detector.initiateExcursion(tracker, measurement, "upward", ask)
		tracker.quietTicks = 0

		return
	}

	if downwardMove >= initiationMove {
		detector.initiateExcursion(tracker, measurement, "downward", bid)
		tracker.quietTicks = 0

		return
	}

	// Flat/choppy detection
	tracker.quietTicks++

	if tracker.quietTicks >= 48 && (measurement.SeqIdx-tracker.lastFlatTick >= 128) {
		detector.sampleFlatSpan(tracker, measurement)
		tracker.quietTicks = 0

		return
	}

	// Dynamic anchor drift when price hovers in quiet baseline
	tracker.anchorPrice = 0.9*tracker.anchorPrice + 0.1*((bid+ask)/2)
}

func (detector *StreamingDetector) initiateExcursion(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
	direction string,
	entryPrice float64,
) {
	// Allocate 20% of currently available balance
	allocatedCapital := detector.availableBalance * 0.20

	if allocatedCapital < 1.0 {
		allocatedCapital = 1.0 // Minimum test allocation
	}

	entryFee := allocatedCapital * detector.feeRate
	detector.availableBalance -= (allocatedCapital + entryFee)

	precursorStart := measurement.SeqIdx

	if len(tracker.precursorTicks) > 0 {
		precursorStart = tracker.precursorTicks[0]
	}

	now := time.Now().UTC()
	tracker.inExcursion = true
	tracker.direction = direction
	tracker.anchorTick = measurement.SeqIdx
	tracker.anchorAt = now
	tracker.anchorPrice = entryPrice
	tracker.precursorStartTick = precursorStart
	tracker.extremumTick = measurement.SeqIdx
	tracker.extremumAt = now
	tracker.extremumPrice = entryPrice
	tracker.entryPrice = entryPrice
	tracker.positionSize = allocatedCapital
	tracker.entryFee = entryFee
	tracker.observationCount = 1
	tracker.exited = false
	tracker.postTicksCollected = 0
	tracker.stagnationCounter = 0
}

func (detector *StreamingDetector) advanceExcursion(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	tracker.observationCount++

	if tracker.exited {
		tracker.postTicksCollected++

		if tracker.postTicksCollected >= detector.postMarginWindow {
			detector.finalizeExcursion(tracker, measurement.SeqIdx)
		}

		return
	}

	bid := tracker.bid

	if bid <= 0 {
		bid = tracker.last
	}

	ask := tracker.ask

	if ask <= 0 {
		ask = tracker.last
	}

	if tracker.direction == "upward" {
		if ask > tracker.extremumPrice {
			tracker.extremumPrice = ask
			tracker.extremumTick = measurement.SeqIdx
			tracker.extremumAt = time.Now().UTC()
			tracker.stagnationCounter = 0

			return
		}

		tracker.stagnationCounter++

		// Retracement check: pull back by 30% of excursion distance
		totalRise := tracker.extremumPrice - tracker.entryPrice

		if totalRise > 0 && bid < tracker.extremumPrice-0.30*totalRise {
			detector.triggerExit(tracker, measurement, bid, "reversed")

			return
		}

		// Stagnation check
		if tracker.stagnationCounter > 24 {
			detector.triggerExit(tracker, measurement, bid, "stagnated")

			return
		}
	}

	if tracker.direction == "downward" {
		if bid < tracker.extremumPrice {
			tracker.extremumPrice = bid
			tracker.extremumTick = measurement.SeqIdx
			tracker.extremumAt = time.Now().UTC()
			tracker.stagnationCounter = 0

			return
		}

		tracker.stagnationCounter++

		totalDrop := tracker.entryPrice - tracker.extremumPrice

		if totalDrop > 0 && ask > tracker.extremumPrice+0.30*totalDrop {
			detector.triggerExit(tracker, measurement, ask, "reversed")

			return
		}

		if tracker.stagnationCounter > 24 {
			detector.triggerExit(tracker, measurement, ask, "stagnated")

			return
		}
	}
}

func (detector *StreamingDetector) triggerExit(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
	exitPrice float64,
	status string,
) {
	tracker.exited = true
	tracker.exitTick = measurement.SeqIdx
	tracker.exitAt = time.Now().UTC()
	tracker.exitPrice = exitPrice
	tracker.exitStatus = status
}

func (detector *StreamingDetector) finalizeExcursion(
	tracker *symbolTracker,
	postEndTick int64,
) {
	exitPrice := tracker.exitPrice
	entryPrice := tracker.entryPrice

	if exitPrice <= 0 {
		exitPrice = entryPrice
	}

	positionSize := tracker.positionSize
	grossProceeds := positionSize

	if entryPrice > 0 {
		grossProceeds = positionSize * (exitPrice / entryPrice)
	}

	exitFee := grossProceeds * detector.feeRate
	totalFee := tracker.entryFee + exitFee
	profit := grossProceeds - positionSize - totalFee

	if tracker.direction == "downward" {
		// Downward excursions simulate loss for longs or negative baseline
		profit = -(grossProceeds - positionSize) - totalFee
	}

	profitFraction := 0.0

	if positionSize > 0 {
		profitFraction = profit / positionSize
	}

	grossExcursion := 0.0

	if tracker.anchorPrice > 0 {
		grossExcursion = (tracker.extremumPrice - tracker.anchorPrice) / tracker.anchorPrice
	}

	clearsFriction := profit > 0
	status := tracker.exitStatus

	if tracker.direction == "upward" && !clearsFriction {
		status = "subfriction"
		detector.subfrictionCount++
	}

	if tracker.direction == "upward" && clearsFriction {
		status = "profitable"
		detector.profitableCount++
	}

	if tracker.direction == "downward" {
		detector.downwardCount++
	}

	// Return allocated capital and realized profit to available balance
	returnedCapital := positionSize + profit

	if returnedCapital < 0 {
		returnedCapital = 0
	}

	detector.availableBalance += returnedCapital

	record := ExcursionRecord{
		Epoch:              detector.epoch,
		ID:                 fmt.Sprintf("%d:%s:%d", detector.epoch, tracker.symbol, tracker.anchorTick),
		Symbol:             tracker.symbol,
		Direction:          tracker.direction,
		ClearsFriction:     clearsFriction,
		PrecursorStartTick: tracker.precursorStartTick,
		AnchorTick:         tracker.anchorTick,
		ExtremumTick:       tracker.extremumTick,
		ExitTick:           tracker.exitTick,
		PostEndTick:        postEndTick,
		EntryPrice:         entryPrice,
		ExtremumPrice:      tracker.extremumPrice,
		ExitPrice:          exitPrice,
		PositionSize:       positionSize,
		Fee:                totalFee,
		Profit:             profit,
		ProfitFraction:     profitFraction,
		GrossExcursion:     grossExcursion,
		ObservationCount:   tracker.observationCount,
		Status:             status,
	}

	if detector.onComplete != nil {
		detector.onComplete(record)
	}

	// Reset tracker state for next excursion
	tracker.inExcursion = false
	tracker.exited = false
	tracker.anchorPrice = exitPrice
	tracker.precursorTicks = tracker.precursorTicks[:0]
}

/*
sampleFlatSpan captures a balanced sample of choppy / flat tape where price moves
only within the noise band, providing negative training evidence where entering
would bleed spread and fees.
*/
func (detector *StreamingDetector) sampleFlatSpan(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	// Quota control: don't let flat tape flood the dataset
	if detector.flatCount*2 > (detector.profitableCount + detector.subfrictionCount + detector.downwardCount + 4) {
		return
	}

	price := tracker.last

	if price <= 0 {
		price = (tracker.bid + tracker.ask) / 2
	}

	if price <= 0 {
		return
	}

	tracker.lastFlatTick = measurement.SeqIdx
	detector.flatCount++

	precursorStart := measurement.SeqIdx - 16

	if len(tracker.precursorTicks) > 0 {
		precursorStart = tracker.precursorTicks[0]
	}

	simulatedSize := detector.availableBalance * 0.20

	if simulatedSize < 1.0 {
		simulatedSize = 1.0
	}

	simulatedFee := 2 * simulatedSize * detector.feeRate
	spreadLoss := 0.0

	if tracker.spread > 0 && price > 0 {
		spreadLoss = simulatedSize * (tracker.spread / price)
	}

	simulatedProfit := -(simulatedFee + spreadLoss)

	record := ExcursionRecord{
		Epoch:              detector.epoch,
		ID:                 fmt.Sprintf("%d:%s:%d:flat", detector.epoch, tracker.symbol, measurement.SeqIdx-32),
		Symbol:             tracker.symbol,
		Direction:          "flat",
		ClearsFriction:     false,
		PrecursorStartTick: precursorStart,
		AnchorTick:         measurement.SeqIdx - 32,
		ExtremumTick:       measurement.SeqIdx - 16,
		ExitTick:           measurement.SeqIdx - 8,
		PostEndTick:        measurement.SeqIdx,
		EntryPrice:         price,
		ExtremumPrice:      price,
		ExitPrice:          price,
		PositionSize:       simulatedSize,
		Fee:                simulatedFee,
		Profit:             simulatedProfit,
		ProfitFraction:     simulatedProfit / simulatedSize,
		GrossExcursion:     0.0,
		ObservationCount:   32,
		Status:             "choppy",
	}

	if detector.onComplete != nil {
		detector.onComplete(record)
	}
}
