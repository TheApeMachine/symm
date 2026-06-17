package replay

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/codec"
	"github.com/theapemachine/symm/signal/toxicity"
)

/*
IngestInstrumentUpdate stores tick sizes used by toxicity and verticality replay.
*/
func IngestInstrumentUpdate(update krakenmarket.InstrumentUpdate) {
	for _, pair := range update.Pairs {
		setTickSize(pair.Symbol, pair.PriceIncrement)
	}
}

/*
IngestBookBatch updates decay features and book-quality payloads in the shared tree.
*/
func IngestBookBatch(tree *dmt.Tree, batch *datura.Artifact) {
	if tree == nil || batch == nil {
		return
	}

	for _, update := range datura.As[krakenmarket.BookUpdates](batch) {
		ingestBookUpdate(tree, update)
	}
}

func ingestBookUpdate(tree *dmt.Tree, update *krakenmarket.BookUpdate) {
	if update == nil || update.Symbol == "" {
		return
	}

	scope := update.Symbol
	observed := update.Timestamp

	if observed.IsZero() {
		observed = time.Now()
	}

	bidDepth, askDepth, spreadBps, mid := bookTouchMetrics(update)

	if mid <= 0 {
		return
	}

	state := decayState(scope)
	state.lastPrice = mid
	state.bidDepths.push(bidDepth)
	state.askDepths.push(askDepth)
	state.densities.push(bidDepth + askDepth)
	state.spreads.push(spreadBps)

	pressure := tradePressure(scope)
	state.pressures.push(pressure)

	denominator := bidDepth + askDepth

	if denominator > 0 {
		state.imbalances.push((bidDepth - askDepth) / denominator)
	}

	insertDecayFeatures(tree, scope, state)
	insertVerticalityBook(tree, scope, spreadBps)

	pair := toxicity.PairFromTick(scope, tickSize(scope))
	toxicity.IngestBook(scope, pair, update, observed)

	if payload, ok := toxicity.ReplayBookPayload(scope); ok {
		insertScopedPayload(tree, "book", scope, codec.EncodePayload(payload...))
	}
}

func bookTouchMetrics(update *krakenmarket.BookUpdate) (bidDepth, askDepth, spreadBps, mid float64) {
	if update == nil {
		return 0, 0, 0, 0
	}

	for _, level := range update.Bids {
		if level.Qty > 0 {
			bidDepth += level.Qty
		}
	}

	for _, level := range update.Asks {
		if level.Qty > 0 {
			askDepth += level.Qty
		}
	}

	if len(update.Bids) == 0 || len(update.Asks) == 0 {
		return bidDepth, askDepth, 0, 0
	}

	bid := update.Bids[0].Price
	ask := update.Asks[0].Price

	if bid <= 0 || ask <= bid {
		return bidDepth, askDepth, 0, 0
	}

	mid = (bid + ask) / 2
	spreadBps = (ask - bid) / mid * 10000.0

	return bidDepth, askDepth, spreadBps, mid
}

func decayPayload(state *decayScopeState) []float64 {
	if state == nil || state.lastPrice <= 0 {
		return nil
	}

	if len(state.bidDepths.values) < 4 {
		return nil
	}

	payload := []float64{state.lastPrice}
	series := [][]float64{
		state.bidDepths.values,
		state.askDepths.values,
		state.densities.values,
		state.spreads.values,
		state.pressures.values,
		state.imbalances.values,
	}

	for _, segment := range series {
		payload = append(payload, float64(len(segment)))
	}

	for _, segment := range series {
		payload = append(payload, segment...)
	}

	encoded := codec.EncodePayload(payload...)

	if !codec.ValidDecayPayload(encoded) {
		return nil
	}

	return payload
}

func insertDecayFeatures(tree *dmt.Tree, scope string, state *decayScopeState) {
	payload := decayPayload(state)

	if payload == nil {
		return
	}

	insertScopedPayload(tree, "features", scope, codec.EncodePayload(payload...))
}

func tradePressure(scope string) float64 {
	raw, ok := tradeWindows.Load(scope)

	if !ok {
		return 0
	}

	window := raw.(*scopedTradeWindow)
	gross := window.buyNotional + window.sellNotional

	if gross <= 0 {
		return 0
	}

	return (window.buyNotional - window.sellNotional) / gross
}

func insertVerticalityBook(tree *dmt.Tree, scope string, spreadBps float64) {
	state := verticalityState(scope)
	state.spreads = appendSeries(state.spreads, spreadBps, verticalityCapacity)

	payload := verticalityPayload(scope, state)

	if payload == nil {
		return
	}

	insertScopedPayload(tree, "book", scope, codec.EncodePayload(payload...))
}

func verticalityPayload(scope string, state *verticalityScopeState) []float64 {
	if state == nil || len(state.prices) < 2 {
		return nil
	}

	firstPrice := state.prices[0]
	lastPrice := state.prices[len(state.prices)-1]

	if firstPrice <= 0 || lastPrice <= 0 {
		return nil
	}

	precursor := math.Abs(lastPrice-firstPrice) / firstPrice
	compression := spreadCompression(state.spreads)

	recentVolume := 0.0

	for _, volume := range state.volumes {
		recentVolume += volume
	}

	rate := recentVolume

	if len(state.volumes) > 0 {
		rate = recentVolume / float64(len(state.volumes))
	}

	if state.baselineRate <= 0 {
		state.baselineRate = rate
	}

	if rate > 0 {
		state.baselineRate = (0.9 * state.baselineRate) + (0.1 * rate)
	}

	rvol := 0.0

	if state.baselineRate > 0 {
		rvol = rate / state.baselineRate
	}

	move := (lastPrice - firstPrice) / firstPrice
	payload := codec.EncodePayload(rvol, precursor, compression, move)

	if !codec.ValidFloatPayload(payload, codec.VerticalityMinFloats) {
		return nil
	}

	return []float64{rvol, precursor, compression, move}
}

func spreadCompression(spreads []float64) float64 {
	if len(spreads) < 2 {
		return 0
	}

	recent := spreads[len(spreads)-1]
	baseline := statistic.MedianOf(spreads)

	if baseline <= 0 || recent >= baseline {
		return 0
	}

	return (baseline - recent) / baseline
}
