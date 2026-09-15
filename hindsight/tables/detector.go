package tables

import (
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
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
	oscillationCount int
	lastDirection  int
	lastFlatTick   int64
	lastChoppyTick int64

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

	// Tape fragment frame buffers
	precursorFrames [][]*data.Measurement[float64]
	excursionFrames [][]*data.Measurement[float64]
	postFrames      [][]*data.Measurement[float64]
}

func newSymbolTracker(symbol string) *symbolTracker {
	return &symbolTracker{
		symbol:          symbol,
		precursorTicks:  make([]int64, 0, 32),
		precursorFrames: make([][]*data.Measurement[float64], 0, 32),
		excursionFrames: make([][]*data.Measurement[float64], 0, 64),
		postFrames:      make([][]*data.Measurement[float64], 0, 16),
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
	onFragment       func(types.ReplayFragment)

	// Balanced training sample quotas
	profitableCount  int64
	subfrictionCount int64
	downwardCount    int64
	flatCount        int64
	choppyCount      int64
}

/*
NewStreamingDetector constructs a StreamingDetector for an epoch and initial cash balance.
*/
func NewStreamingDetector(
	epoch int64,
	initialBalance float64,
	onComplete func(ExcursionRecord),
	onFragment ...func(types.ReplayFragment),
) *StreamingDetector {
	balance := initialBalance

	if balance <= 0 {
		balance = 200.0 // Canonical initial paper/test balance
	}

	var fragmentSink func(types.ReplayFragment)

	if len(onFragment) > 0 {
		fragmentSink = onFragment[0]
	}

	return &StreamingDetector{
		epoch:            epoch,
		availableBalance: balance,
		feeRate:          0.0026, // Authoritative Kraken taker fee rate
		precursorWindow:  16,
		postMarginWindow: 8,
		trackers:         make(map[string]*symbolTracker),
		onComplete:       onComplete,
		onFragment:       fragmentSink,
	}
}

/*
SetFragmentSink configures a downstream consumer for complete tape fragments.
*/
func (detector *StreamingDetector) SetFragmentSink(sink func(types.ReplayFragment)) {
	detector.mutex.Lock()
	defer detector.mutex.Unlock()

	detector.onFragment = sink
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

	cloned := cloneMeasurement(measurement)
	frame := []*data.Measurement[float64]{cloned}

	if tracker.inExcursion {
		if !tracker.exited {
			tracker.excursionFrames = append(tracker.excursionFrames, frame)
		}

		if tracker.exited {
			tracker.postFrames = append(tracker.postFrames, frame)
		}

		detector.advanceExcursion(tracker, measurement)

		return
	}

	tracker.precursorFrames = append(tracker.precursorFrames, frame)

	if len(tracker.precursorFrames) > detector.precursorWindow {
		tracker.precursorFrames = tracker.precursorFrames[1:]
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

	// Flat/choppy baseline observation
	tracker.quietTicks++

	// Directional oscillation tracking in quiet baseline
	midPrice := (bid + ask) / 2
	diff := midPrice - tracker.anchorPrice
	noiseThreshold := 0.0008 * tracker.anchorPrice

	if diff > -noiseThreshold && diff < noiseThreshold {
		currDir := 0

		if diff > 0.0001*tracker.anchorPrice {
			currDir = 1
		}

		if diff < -0.0001*tracker.anchorPrice {
			currDir = -1
		}

		if currDir != 0 && currDir != tracker.lastDirection {
			tracker.oscillationCount++
			tracker.lastDirection = currDir
		}
	}

	// Choppy tape: multiple directional flips inside noise band
	if tracker.oscillationCount >= 4 && (measurement.SeqIdx-tracker.lastChoppyTick >= 128) {
		detector.sampleChoppySpan(tracker, measurement)
		tracker.oscillationCount = 0
		tracker.quietTicks = 0

		return
	}

	// Flat tape: prolonged quietness with minimal oscillation
	if tracker.quietTicks >= 48 && tracker.oscillationCount < 4 && (measurement.SeqIdx-tracker.lastFlatTick >= 128) {
		detector.sampleFlatSpan(tracker, measurement)
		tracker.quietTicks = 0
		tracker.oscillationCount = 0

		return
	}

	// Dynamic anchor drift when price hovers in quiet baseline
	tracker.anchorPrice = 0.9*tracker.anchorPrice + 0.1*midPrice
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
	profitFraction := 0.0

	if positionSize > 0 {
		profitFraction = profit / positionSize
	}

	grossExcursion := 0.0

	if tracker.anchorPrice > 0 {
		grossExcursion = (tracker.extremumPrice - tracker.anchorPrice) / tracker.anchorPrice
	}

	// Return allocated capital and realized profit to available balance unconditionally
	returnedCapital := positionSize + profit

	if returnedCapital < 0 {
		returnedCapital = 0
	}

	detector.availableBalance += returnedCapital

	// Excursion must have minimum duration/observations to form a valid training fragment
	if tracker.observationCount < 6 {
		detector.resetTracker(tracker, exitPrice)

		return
	}

	clearsFriction := tracker.direction == "upward" && profit > 0
	status := tracker.exitStatus
	shouldStore := false

	if tracker.direction == "upward" && clearsFriction {
		status = "profitable"
		detector.profitableCount++
		shouldStore = true
	}

	if tracker.direction == "upward" && !clearsFriction {
		status = "subfriction"

		// Only store if within balanced quota and had a noticeable departure
		if detector.subfrictionCount < (detector.profitableCount+4) && tracker.extremumPrice > tracker.entryPrice*1.0008 {
			detector.subfrictionCount++
			shouldStore = true
		}
	}

	if tracker.direction == "downward" {
		status = "declining"

		// Only store if within balanced quota
		if detector.downwardCount < (detector.profitableCount+4) {
			detector.downwardCount++
			shouldStore = true
		}
	}

	if shouldStore {
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

		if detector.onFragment != nil {
			allFrames := make([][]*data.Measurement[float64], 0, len(tracker.precursorFrames)+len(tracker.excursionFrames)+len(tracker.postFrames))
			allFrames = append(allFrames, tracker.precursorFrames...)
			allFrames = append(allFrames, tracker.excursionFrames...)
			allFrames = append(allFrames, tracker.postFrames...)

			anchorIdx := len(tracker.precursorFrames)
			extremumOffset := int(tracker.extremumTick - tracker.anchorTick)
			extremumIdx := anchorIdx + extremumOffset

			if extremumIdx >= len(allFrames) {
				extremumIdx = len(allFrames) - 1
			}

			if tracker.direction == "downward" {
				anchorIdx = -1
				extremumIdx = -1
			}

			detector.onFragment(types.ReplayFragment{
				Frames:        allFrames,
				Symbol:        tracker.symbol,
				AnchorIndex:   anchorIdx,
				ExtremumIndex: extremumIdx,
			})
		}
	}

	detector.resetTracker(tracker, exitPrice)
}

func (detector *StreamingDetector) resetTracker(tracker *symbolTracker, exitPrice float64) {
	tracker.inExcursion = false
	tracker.exited = false
	tracker.anchorPrice = exitPrice
	tracker.precursorTicks = tracker.precursorTicks[:0]
	tracker.precursorFrames = tracker.precursorFrames[:0]
	tracker.excursionFrames = tracker.excursionFrames[:0]
	tracker.postFrames = tracker.postFrames[:0]
	tracker.quietTicks = 0
	tracker.oscillationCount = 0
}

/*
sampleFlatSpan captures a balanced sample of quiet, flat tape where price hovers
in a tight noise band with minimal oscillation, providing negative training
evidence where waiting is reinforced and entering bleeds fees.
*/
func (detector *StreamingDetector) sampleFlatSpan(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	totalNonFlat := detector.profitableCount + detector.subfrictionCount + detector.downwardCount

	if detector.flatCount >= (totalNonFlat/3 + 4) {
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
		Status:             "flat",
	}

	if detector.onComplete != nil {
		detector.onComplete(record)
	}

	if detector.onFragment != nil {
		allFrames := make([][]*data.Measurement[float64], 0, len(tracker.precursorFrames)+1)
		allFrames = append(allFrames, tracker.precursorFrames...)
		allFrames = append(allFrames, []*data.Measurement[float64]{cloneMeasurement(measurement)})

		detector.onFragment(types.ReplayFragment{
			Frames:        allFrames,
			Symbol:        tracker.symbol,
			AnchorIndex:   -1,
			ExtremumIndex: -1,
		})
	}
}

/*
sampleChoppySpan captures a balanced sample of choppy, oscillating tape where
price thrashes across baseline without clearing friction, providing negative
training evidence where whipsaws and spread bleed are penalized.
*/
func (detector *StreamingDetector) sampleChoppySpan(
	tracker *symbolTracker,
	measurement *data.Measurement[float64],
) {
	totalNonFlat := detector.profitableCount + detector.subfrictionCount + detector.downwardCount

	if detector.choppyCount >= (totalNonFlat/3 + 4) {
		return
	}

	price := tracker.last

	if price <= 0 {
		price = (tracker.bid + tracker.ask) / 2
	}

	if price <= 0 {
		return
	}

	tracker.lastChoppyTick = measurement.SeqIdx
	detector.choppyCount++

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
		ID:                 fmt.Sprintf("%d:%s:%d:choppy", detector.epoch, tracker.symbol, measurement.SeqIdx-32),
		Symbol:             tracker.symbol,
		Direction:          "choppy",
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

	if detector.onFragment != nil {
		allFrames := make([][]*data.Measurement[float64], 0, len(tracker.precursorFrames)+1)
		allFrames = append(allFrames, tracker.precursorFrames...)
		allFrames = append(allFrames, []*data.Measurement[float64]{cloneMeasurement(measurement)})

		detector.onFragment(types.ReplayFragment{
			Frames:        allFrames,
			Symbol:        tracker.symbol,
			AnchorIndex:   -1,
			ExtremumIndex: -1,
		})
	}
}

/*
Counts returns current category tallies across stored excursions.
*/
func (detector *StreamingDetector) Counts() (profitable, subfriction, downward, flat, choppy int64) {
	detector.mutex.Lock()
	defer detector.mutex.Unlock()

	return detector.profitableCount, detector.subfrictionCount, detector.downwardCount, detector.flatCount, detector.choppyCount
}

/*
AvailableBalance returns current unallocated trading balance.
*/
func (detector *StreamingDetector) AvailableBalance() float64 {
	detector.mutex.Lock()
	defer detector.mutex.Unlock()

	return detector.availableBalance
}

func cloneMeasurement(measured *data.Measurement[float64]) *data.Measurement[float64] {
	if measured == nil {
		return nil
	}

	metrics := make(map[string]data.Metric[float64], len(measured.Metrics))

	for key, value := range measured.Metrics {
		metrics[key] = value
	}

	meta := make(map[string]string, len(measured.Metadata))

	for key, value := range measured.Metadata {
		meta[key] = value
	}

	prov := make(map[string]string, len(measured.Provenance))

	for key, value := range measured.Provenance {
		prov[key] = value
	}

	peers := make([]*data.Measurement[float64], len(measured.Peers))

	for idx, peer := range measured.Peers {
		peers[idx] = cloneMeasurement(peer)
	}

	return &data.Measurement[float64]{
		ID:         measured.ID,
		Label:      measured.Label,
		Source:     measured.Source,
		SeqIdx:     measured.SeqIdx,
		At:         measured.At,
		From:       measured.From,
		Maturity:   measured.Maturity,
		SNR:        measured.SNR,
		SNRDefined: measured.SNRDefined,
		Estimated:  measured.Estimated,
		Err:        measured.Err,
		Metrics:    metrics,
		Metadata:   meta,
		Provenance: prov,
		Peers:      peers,
	}
}
