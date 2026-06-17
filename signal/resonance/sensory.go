package resonance

import (
	"math"

	"github.com/theapemachine/nomagique/statistic"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

const bookDepthLevels = 5

type marketFacts struct {
	lastPrice       float64
	volume          float64
	spreadBps       float64
	elapsed         float64
	changeAbs       float64
	changePct       float64
	buyPressure     float64
	tradeRate       float64
	tradeNotional   float64
	touchImbalance  float64
	depthImbalance  float64
	spreadWideRatio float64
	tickCadence     float64
	midDriftBps     float64
}

func buildSensoryVector(
	symbol string,
	ticker *feed.Ticker,
	book *feed.Book,
	trade *feed.Trade,
	registry *senseRegistry,
) ([]float64, marketFacts, bool) {
	tickerSnap := ticker.Snapshot(symbol)

	if tickerSnap.Last <= 0 {
		return nil, marketFacts{}, false
	}

	facts := marketFacts{
		lastPrice: tickerSnap.Last,
		volume:    tickerSnap.Volume,
		spreadBps: book.Spread(symbol),
		elapsed:   tickerSnap.Elapsed,
		changeAbs: math.Abs(tickerSnap.ChangePct),
		changePct: tickerSnap.ChangePct,
	}

	if bookWindow, ok := book.Window(symbol); ok && bookWindow.Latest != nil {
		facts.touchImbalance = touchImbalance(bookWindow.Latest)
		facts.depthImbalance = depthImbalance(bookWindow.Latest, bookDepthLevels)
		facts.spreadWideRatio = spreadWideRatio(book.Spread(symbol), bookWindow.Spreads)
		facts.midDriftBps = midDriftBps(tickerSnap.Last, bookWindow.Latest)
	}

	if tradeWindow, ok := trade.Window(symbol); ok {
		flow := tradeFlow(symbol, trade)

		facts.buyPressure = flow.buyPressure
		facts.tradeRate = flow.tradeRate
		facts.tradeNotional = flow.notionalRate

		if tradeWindow.Elapsed > 0 {
			facts.tickCadence = 1 / math.Max(tradeWindow.Elapsed, 1e-3)
		}
	}

	if facts.elapsed > 0 {
		facts.tickCadence = math.Max(facts.tickCadence, 1/facts.elapsed)
	}

	baselines := registry.baselines(symbol)
	vector := []float64{
		scaledSigned(facts.changePct, &baselines.changeAbs),
		ratioToMedian(facts.spreadBps, &baselines.spreadBps),
		ratioToMedian(math.Log1p(math.Max(facts.volume, 0)), &baselines.logVolume),
		ratioToMedian(facts.tradeRate, &baselines.tradeRate),
		scaledSigned(facts.buyPressure, &baselines.buyPressure),
		scaledSigned(facts.touchImbalance, &baselines.touchImbal),
		scaledSigned(facts.depthImbalance, &baselines.depthImbal),
		ratioToMedian(facts.spreadWideRatio, &baselines.spreadWide),
		ratioToMedian(facts.tickCadence, &baselines.tickCadence),
		ratioToMedian(facts.tradeNotional, &baselines.tradeNotional),
		scaledSigned(facts.midDriftBps, &baselines.midDrift),
		scaledSigned(facts.changeAbs, &baselines.changeAbs),
	}

	for index, value := range vector {
		if !logic.ScalarFinite(value) {
			vector[index] = 0
		}
	}

	if len(vector) != SensoryChannelCount {
		return nil, marketFacts{}, false
	}

	return vector, facts, true
}

func touchImbalance(update *krakenmarket.BookUpdate) float64 {
	if update == nil || len(update.Bids) == 0 || len(update.Asks) == 0 {
		return 0
	}

	bidQty := update.Bids[0].Qty
	askQty := update.Asks[0].Qty
	total := bidQty + askQty

	if total <= 0 {
		return 0
	}

	return (bidQty - askQty) / total
}

func depthImbalance(update *krakenmarket.BookUpdate, levels int) float64 {
	if update == nil || levels <= 0 {
		return 0
	}

	bidQty := 0.0
	askQty := 0.0

	for index := 0; index < levels && index < len(update.Bids); index++ {
		if update.Bids[index].Qty > 0 {
			bidQty += update.Bids[index].Qty
		}
	}

	for index := 0; index < levels && index < len(update.Asks); index++ {
		if update.Asks[index].Qty > 0 {
			askQty += update.Asks[index].Qty
		}
	}

	total := bidQty + askQty

	if total <= 0 {
		return 0
	}

	return (bidQty - askQty) / total
}

func spreadWideRatio(currentSpreadBps float64, spreads []float64) float64 {
	if currentSpreadBps <= 0 || len(spreads) == 0 {
		return 0
	}

	reference := statistic.QuantileOf(0.75, spreads)

	if reference <= 1e-12 {
		return 1
	}

	return currentSpreadBps / reference
}

func midDriftBps(lastPrice float64, update *krakenmarket.BookUpdate) float64 {
	if update == nil || len(update.Bids) == 0 || len(update.Asks) == 0 || lastPrice <= 0 {
		return 0
	}

	mid := (update.Bids[0].Price + update.Asks[0].Price) / 2
	spread := update.Asks[0].Price - update.Bids[0].Price

	if spread <= 0 {
		return 0
	}

	return ((lastPrice - mid) / spread) * 10000
}

type tradeFlowStats struct {
	buyPressure  float64
	tradeRate    float64
	notionalRate float64
}

func tradeFlow(symbol string, trade *feed.Trade) tradeFlowStats {
	stats := tradeFlowStats{}

	if !trade.Scan(symbol, func(update *krakenmarket.TradeUpdate) {
		if update == nil || update.Price <= 0 || update.Qty <= 0 {
			return
		}

		stats.tradeRate++

		notional := update.Price * update.Qty
		stats.notionalRate += notional

		if update.Side == "buy" {
			stats.buyPressure += notional
		}

		if update.Side == "sell" {
			stats.buyPressure -= notional
		}
	}) {
		return stats
	}

	window, ok := trade.Window(symbol)

	if !ok || window.Elapsed <= 0 {
		return stats
	}

	stats.tradeRate /= window.Elapsed
	stats.notionalRate /= window.Elapsed

	gross := math.Abs(stats.buyPressure)

	if gross <= 0 {
		stats.buyPressure = 0

		return stats
	}

	stats.buyPressure /= gross

	return stats
}
