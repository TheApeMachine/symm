package leadlag

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/correlation"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric/learned"
)

const (
	priceHistoryCap = 256
	maxLagBars      = 12
	minLagSamples   = 16
)

/*
symbolState tracks the rolling price path needed to compute a real
cross-correlation against the anchor pair. The previous implementation
stored only the most recent change_pct, which could not produce a lag —
two scalars at the same instant carry no information about ordering. The
ring buffer here records (timestamp, last) pairs, sampled on every
ticker frame, so lagBars/lagMaxCorr can measure leadership in bars rather
than in 24-hour dispersion.
*/
type symbolState struct {
	mu        sync.RWMutex
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
	prices    correlation.PriceSampleRing
	forecast  *learned.Forecast
}

type symbolSnapshot struct {
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
	scale     float64
}

func newSymbolState(pair asset.Pair) *symbolState {
	return &symbolState{
		pair:     pair,
		prices:   correlation.NewPriceSampleRing(priceHistoryCap),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *symbolState) observe(at time.Time, price float64) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.prices.Push(at, price)
}

func (state *symbolState) observeTicker(
	changePct float64,
	last float64,
	bid float64,
	ask float64,
	at time.Time,
) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = changePct
	state.last = last

	if bid > 0 {
		state.bid = bid
	}

	if ask > 0 {
		state.ask = ask
	}

	state.prices.Push(at, last)
}

func (state *symbolState) snapshot() symbolSnapshot {
	state.mu.RLock()
	defer state.mu.RUnlock()

	scale := 1.0

	if state.forecast != nil {
		scale = state.forecast.Scale()
	}

	return symbolSnapshot{
		pair:      state.pair,
		changePct: state.changePct,
		last:      state.last,
		bid:       state.bid,
		ask:       state.ask,
		scale:     scale,
	}
}

func (state *symbolState) priceSamples() []correlation.PriceSample {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.prices.Ordered()
}

func (state *symbolState) change() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.changePct
}

func (state *symbolState) forecastLearner() *learned.Forecast {
	if state.forecast == nil {
		state.forecast = learned.NewForecast(0.35)
	}

	return state.forecast
}

func (state *symbolState) forecastScale() float64 {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.forecastLearner().Scale()
}

func (state *symbolState) applyFeedback(predictedReturn, actualReturn float64) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	_, err := state.forecastLearner().Next(0, predictedReturn, actualReturn)

	return err
}

/*
crossLag computes the bar lag at which anchor's returns most strongly
predict state's returns over the configured correlation window. Positive
lag means anchor leads state by lag bars; negative means state leads
anchor (and is therefore not a lead-lag opportunity in this direction).
Returns (lagBars, correlation, ok); ok is false when there is insufficient
overlap to compute a reliable estimate.
*/
func crossLag(anchor, state *symbolState) (int, float64, bool) {
	anchorSeries := anchor.priceSamples()
	stateSeries := state.priceSamples()

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, 0, false
	}

	bestCorr := 0.0
	bestLag := 0
	interval := correlation.BarInterval()

	if corr, ok := correlation.HayashiYoshidaCorrelation(anchorSeries, stateSeries); ok && corr > 0 {
		bestCorr = corr
	}

	for lag := 1; lag <= maxLagBars; lag++ {
		shiftedAnchor := correlation.ShiftPriceSamples(
			anchorSeries,
			time.Duration(lag)*interval,
		)
		corr, ok := correlation.HayashiYoshidaCorrelation(shiftedAnchor, stateSeries)

		if ok && corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	if bestLag <= 0 || bestCorr <= 0 {
		return 0, 0, false
	}

	return bestLag, bestCorr, true
}
