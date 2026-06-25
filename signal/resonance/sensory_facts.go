package resonance

import (
	"math"
)

func buildSensoryVector(
	symbol string,
	ticker *marketTicker,
	book *marketBook,
	trade *marketTrade,
	registry *senseRegistry,
) ([]float64, marketFacts, bool) {
	facts, ok := collectMarketFacts(symbol, ticker, book, trade)

	if !ok {
		return nil, marketFacts{}, false
	}

	vector, ok := assembleSensoryVector(symbol, facts, registry, facts.observedStamp)

	if !ok {
		return nil, marketFacts{}, false
	}

	return vector, facts, true
}

func collectMarketFacts(
	symbol string,
	ticker *marketTicker,
	book *marketBook,
	trade *marketTrade,
) (marketFacts, bool) {
	tickerSnap := ticker.Snapshot(symbol)

	if tickerSnap.Last <= 0 {
		return marketFacts{}, false
	}

	facts := marketFacts{
		lastPrice:     tickerSnap.Last,
		volume:        tickerSnap.Volume,
		spreadBps:     book.Spread(symbol),
		elapsed:       tickerSnap.Elapsed,
		changeAbs:     math.Abs(tickerSnap.ChangePct),
		changePct:     tickerSnap.ChangePct,
		observedStamp: float64(tickerSnap.Observed.UnixNano()),
	}

	enrichFactsFromBook(&facts, symbol, book, tickerSnap.Last)
	enrichFactsFromTrade(&facts, symbol, trade)

	if facts.elapsed > 0 {
		facts.tickCadence = math.Max(facts.tickCadence, 1/facts.elapsed)
	}

	return facts, true
}

func enrichFactsFromBook(
	facts *marketFacts,
	symbol string,
	book *marketBook,
	lastPrice float64,
) {
	bookWindow, ok := book.Window(symbol)

	if !ok || len(bookWindow.LatestElement) == 0 {
		return
	}

	facts.touchImbalance = touchImbalance(bookWindow.LatestElement)
	facts.depthImbalance = depthImbalance(bookWindow.LatestElement, bookDepthLimit(bookWindow.LatestElement))
	facts.spreadWideRatio = spreadWideRatio(book.Spread(symbol), bookWindow.Spreads)
	facts.midDriftBps = midDriftBps(lastPrice, bookWindow.LatestElement)
}

func enrichFactsFromTrade(
	facts *marketFacts,
	symbol string,
	trade *marketTrade,
) {
	tradeWindow, ok := trade.Window(symbol)

	if !ok {
		return
	}

	flow := tradeFlow(symbol, trade)

	facts.buyPressure = flow.buyPressure
	facts.tradeRate = flow.tradeRate
	facts.tradeNotional = flow.notionalRate

	if tradeWindow.Elapsed > 0 {
		facts.tickCadence = 1 / math.Max(tradeWindow.Elapsed, 1e-3)
	}
}

func assembleSensoryVector(
	symbol string,
	facts marketFacts,
	registry *senseRegistry,
	stamp float64,
) ([]float64, bool) {
	baselines := registry.baselines(symbol)
	vector := []float64{
		scaledSigned(facts.changePct, &baselines.changeAbs, stamp),
		spreadRatioToMedian(facts.spreadBps, &baselines.spreadBps, stamp),
		ratioToMedian(math.Log1p(math.Max(facts.volume, 0)), &baselines.logVolume, stamp),
		ratioToMedian(facts.tradeRate, &baselines.tradeRate, stamp),
		scaledSigned(facts.buyPressure, &baselines.buyPressure, stamp),
		scaledSigned(facts.touchImbalance, &baselines.touchImbal, stamp),
		scaledSigned(facts.depthImbalance, &baselines.depthImbal, stamp),
		ratioToMedian(facts.spreadWideRatio, &baselines.spreadWide, stamp),
		ratioToMedian(facts.tickCadence, &baselines.tickCadence, stamp),
		ratioToMedian(facts.tradeNotional, &baselines.tradeNotional, stamp),
		scaledSigned(facts.midDriftBps, &baselines.midDrift, stamp),
		scaledSigned(facts.changeAbs, &baselines.changeAbs, stamp),
	}

	for index, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			vector[index] = 0
		}
	}

	if len(vector) != SensoryChannelCount {
		return nil, false
	}

	return vector, true
}
