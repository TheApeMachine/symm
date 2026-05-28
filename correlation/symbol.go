package correlation

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/learned"
)

const correlationSource = "correlation"

type symbolState struct {
	mu       sync.RWMutex
	pair     asset.Pair
	window   PriceSampleRing
	last     float64
	bid      float64
	ask      float64
	forecast *learned.Forecast
}

type symbolSnapshot struct {
	pair    asset.Pair
	samples []PriceSample
	last    float64
	bid     float64
	ask     float64
	scale   float64
}

func newSymbolState(pair asset.Pair, windowCap int) *symbolState {
	return &symbolState{
		pair:     pair,
		window:   NewPriceSampleRing(windowCap),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *symbolState) snapshot() symbolSnapshot {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return symbolSnapshot{
		pair:    state.pair,
		samples: state.window.Ordered(),
		last:    state.last,
		bid:     state.bid,
		ask:     state.ask,
		scale:   forecastScale(state.forecast),
	}
}

func forecastScale(forecast *learned.Forecast) float64 {
	if forecast == nil {
		return 1
	}

	return forecast.Scale()
}

func (state *symbolState) observeTick(row market.TickerRow, at time.Time) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if row.Last > 0 {
		state.last = row.Last
		state.window.Push(at, row.Last)
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}
}

func (state *symbolState) applyFeedback(predictedReturn, actualReturn float64) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.forecast == nil {
		state.forecast = learned.NewForecast(0.35)
	}

	_, err := state.forecast.Next(0, predictedReturn, actualReturn)

	return err
}

func pairCorrelation(
	leftState *symbolState,
	rightState *symbolState,
	minSamples int,
) (float64, bool) {
	leftSnapshot := leftState.snapshot()
	rightSnapshot := rightState.snapshot()

	if len(leftSnapshot.samples) < minSamples || len(rightSnapshot.samples) < minSamples {
		return 0, false
	}

	return HayashiYoshidaCorrelation(leftSnapshot.samples, rightSnapshot.samples)
}

func correlationMeasurement(
	state *symbolState,
	peakScore float64,
) (engine.Measurement, bool) {
	snapshot := state.snapshot()

	if peakScore <= 0 || snapshot.last <= 0 {
		return engine.Measurement{}, false
	}

	confidence := engine.ConfidenceFromScore(peakScore * snapshot.scale)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Basis,
		Source:     correlationSource,
		Regime:     "correlation",
		Reason:     "pair_correlation",
		Pairs:      []asset.Pair{snapshot.pair},
		Confidence: confidence,
		Last:       snapshot.last,
		Bid:        snapshot.bid,
		Ask:        snapshot.ask,
	}, true
}

func windowCap() int {
	windowCap := config.System.MinCorrelationSamples

	if config.System.PriceHistory > 0 && config.System.PriceHistory < windowCap {
		windowCap = config.System.PriceHistory
	}

	return windowCap
}
