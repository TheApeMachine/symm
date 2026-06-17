package resonance

import (
	"math"

	"github.com/theapemachine/nomagique/statistic"
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

	if bookWindow, ok := book.Window(symbol); ok && len(bookWindow.LatestElement) > 0 {
		facts.touchImbalance = touchImbalance(bookWindow.LatestElement)
		facts.depthImbalance = depthImbalance(bookWindow.LatestElement, bookDepthLevels)
		facts.spreadWideRatio = spreadWideRatio(book.Spread(symbol), bookWindow.Spreads)
		facts.midDriftBps = midDriftBps(tickerSnap.Last, bookWindow.LatestElement)
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

func touchImbalance(element []byte) float64 {
	bidQty, bidOK := feed.PeekElementOK[float64](element, "bids.0.qty")
	askQty, askOK := feed.PeekElementOK[float64](element, "asks.0.qty")

	if !bidOK || !askOK {
		return 0
	}

	total := bidQty + askQty

	if total <= 0 {
		return 0
	}

	return (bidQty - askQty) / total
}

func depthImbalance(element []byte, levels int) float64 {
	if len(element) == 0 || levels <= 0 {
		return 0
	}

	bidQty := 0.0
	askQty := 0.0
	bidIndex := 0
	askIndex := 0

	feed.EachBookLevelElement(element, "bids", func(price float64, qty float64) {
		if bidIndex >= levels {
			return
		}

		if qty > 0 {
			bidQty += qty
		}

		bidIndex++
	})

	feed.EachBookLevelElement(element, "asks", func(price float64, qty float64) {
		if askIndex >= levels {
			return
		}

		if qty > 0 {
			askQty += qty
		}

		askIndex++
	})

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

func midDriftBps(lastPrice float64, element []byte) float64 {
	bidPrice, bidOK := feed.PeekElementOK[float64](element, "bids.0.price")
	askPrice, askOK := feed.PeekElementOK[float64](element, "asks.0.price")

	if !bidOK || !askOK || lastPrice <= 0 {
		return 0
	}

	mid := (bidPrice + askPrice) / 2
	spread := askPrice - bidPrice

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

	if !trade.Scan(symbol, func(element []byte) {
		price, priceOK := feed.PeekElementOK[float64](element, "price")
		qty, qtyOK := feed.PeekElementOK[float64](element, "qty")

		if !priceOK || !qtyOK || price <= 0 || qty <= 0 {
			return
		}

		stats.tradeRate++

		notional := price * qty
		stats.notionalRate += notional

		side, _ := feed.PeekElementOK[string](element, "side")

		if side == "buy" {
			stats.buyPressure += notional
		}

		if side == "sell" {
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
