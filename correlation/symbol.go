package correlation

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/learned"
)

const correlationSource = "correlation"

type symbolState struct {
	pair     asset.Pair
	window   PriceSampleRing
	last     float64
	bid      float64
	ask      float64
	forecast *learned.Forecast
}

func newSymbolState(pair asset.Pair, windowCap int) *symbolState {
	return &symbolState{
		pair:     pair,
		window:   NewPriceSampleRing(windowCap),
		forecast: learned.NewForecast(0.35),
	}
}

func (symbolState *symbolState) forecastScale() float64 {
	if symbolState.forecast == nil {
		return 1
	}

	return symbolState.forecast.Scale()
}

func (symbolState *symbolState) observeTick(row market.TickerRow, at time.Time) {
	if row.Last > 0 {
		symbolState.last = row.Last
		symbolState.window.Push(at, row.Last)
	}

	if row.Bid > 0 {
		symbolState.bid = row.Bid
	}

	if row.Ask > 0 {
		symbolState.ask = row.Ask
	}
}

func pairCorrelation(
	leftState *symbolState,
	rightState *symbolState,
	minSamples int,
) (float64, bool) {
	leftReturns, rightReturns, ok := SynchronizedLogReturns(
		leftState.window.Ordered(),
		rightState.window.Ordered(),
		BarInterval(),
	)

	if !ok || len(leftReturns) < minSamples {
		return 0, false
	}

	return Pearson(leftReturns, rightReturns), true
}

func correlationMeasurement(
	state *symbolState,
	peakScore float64,
) (engine.Measurement, bool) {
	if peakScore <= 0 || state.last <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.ConfidenceFromScore(peakScore * state.forecastScale())

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Basis,
		Source:     correlationSource,
		Regime:     "correlation",
		Reason:     "pair_correlation",
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.last,
		Bid:        state.bid,
		Ask:        state.ask,
	}, true
}

func windowCap() int {
	windowCap := config.System.MinCorrelationSamples

	if config.System.PriceHistory > 0 && config.System.PriceHistory < windowCap {
		windowCap = config.System.PriceHistory
	}

	return windowCap
}
