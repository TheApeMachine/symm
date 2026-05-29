package sentiment

import (
	"sync"

	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/learned"
)

type symbolState struct {
	mu        sync.RWMutex
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
	forecast  *learned.Forecast
}

func newSymbolState(pair asset.Pair) *symbolState {
	return &symbolState{
		pair:     pair,
		forecast: learned.NewForecast(0.35),
	}
}

type symbolSnapshot struct {
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
}

func (state *symbolState) observeTicker(row market.TickerRow) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = row.ChangePct

	if row.Last > 0 {
		state.last = row.Last
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}
}

func (state *symbolState) snapshot() symbolSnapshot {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return symbolSnapshot{
		pair:      state.pair,
		changePct: state.changePct,
		last:      state.last,
		bid:       state.bid,
		ask:       state.ask,
	}
}

func (state *symbolState) forecastLearner() *learned.Forecast {
	if state.forecast == nil {
		state.forecast = learned.NewForecast(0.35)
	}

	return state.forecast
}

func (state *symbolState) forecastScaleLocked() float64 {
	return state.forecastLearner().Scale()
}

func (state *symbolState) calibratedConfidence(confidence float64) float64 {
	state.mu.Lock()
	defer state.mu.Unlock()

	if confidence <= 0 {
		return 0
	}

	scale := state.forecastScaleLocked()

	if scale <= 0 {
		return 0
	}

	scaled := confidence * scale

	if scaled > 1 {
		return 1
	}

	return scaled
}

func (state *symbolState) applyFeedback(predictedReturn, actualReturn float64) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	_, err := state.forecastLearner().Next(0, predictedReturn, actualReturn)

	return err
}
