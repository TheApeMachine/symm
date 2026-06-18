package resonance

import (
	"math"

	"github.com/theapemachine/symm/logic"
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

	vector, ok := assembleSensoryVector(symbol, facts, registry)

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
		lastPrice: tickerSnap.Last,
		volume:    tickerSnap.Volume,
		spreadBps: book.Spread(symbol),
		elapsed:   tickerSnap.Elapsed,
		changeAbs: math.Abs(tickerSnap.ChangePct),
		changePct: tickerSnap.ChangePct,
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
	facts.depthImbalance = depthImbalance(bookWindow.LatestElement, bookDepthLevels)
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
) ([]float64, bool) {
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
		return nil, false
	}

	return vector, true
}
