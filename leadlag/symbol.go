package leadlag

import (
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
	pair      asset.Pair
	changePct float64
	last      float64
	bid       float64
	ask       float64
	prices    correlation.PriceSampleRing
	forecast  *learned.Forecast
}

func newSymbolState(pair asset.Pair) *symbolState {
	return &symbolState{
		pair:     pair,
		prices:   correlation.NewPriceSampleRing(priceHistoryCap),
		forecast: learned.NewForecast(0.35),
	}
}

func (state *symbolState) observe(at time.Time, price float64) {
	state.prices.Push(at, price)
}

func (state *symbolState) forecastLearner() *learned.Forecast {
	if state.forecast == nil {
		state.forecast = learned.NewForecast(0.35)
	}

	return state.forecast
}

func (state *symbolState) forecastScale() float64 {
	return state.forecastLearner().Scale()
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
	anchorSeries := anchor.prices.Ordered()
	stateSeries := state.prices.Ordered()

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, 0, false
	}

	anchorReturns, stateReturns, ok := correlation.SynchronizedLogReturns(
		anchorSeries, stateSeries, correlation.BarInterval(),
	)

	if !ok || len(anchorReturns) < minLagSamples {
		return 0, 0, false
	}

	bestLag := 0
	bestCorr := correlation.Pearson(anchorReturns, stateReturns)

	for lag := 1; lag <= maxLagBars; lag++ {
		if lag >= len(anchorReturns) {
			break
		}

		// Positive lag: state's return at t is correlated with anchor's
		// return at t-lag. We slice the anchor older and state newer.
		anchorLead := anchorReturns[:len(anchorReturns)-lag]
		stateFollow := stateReturns[lag:]

		corr := correlation.Pearson(anchorLead, stateFollow)

		if corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	if bestCorr <= 0 {
		return 0, 0, false
	}

	return bestLag, bestCorr, true
}

