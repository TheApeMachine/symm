package leadlag

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	priceHistoryCap              = 256
	maxLagBars                   = 12
	minLagSamples                = 16
	leadlagDominanceMarginAbs    = 0.1
	leadlagDominanceMarginRel    = 0.15
	leadlagMinimumLagCorrelation = 0.1
)

/*
symbolState tracks the rolling price path needed to compute a real
cross-correlation against the anchor pair. The ring records (timestamp, price)
pairs sampled on every ticker frame, so the lag can be measured in bars rather
than in a same-instant cross-section spread.
*/
type symbolState struct {
	mu        sync.RWMutex
	changePct float64
	last      float64
	prices    numeric.PriceSampleRing
	floor     *adaptive.SNR
}

func newSymbolState() *symbolState {
	return &symbolState{
		prices: numeric.NewPriceSampleRing(priceHistoryCap),
		floor:  adaptive.NewSNR(),
	}
}

func (state *symbolState) observeTicker(changePct, last float64, at time.Time) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.changePct = changePct
	state.last = last
	state.prices.Push(at, last)
}

func (state *symbolState) priceSamples() []numeric.PriceSample {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.prices.Ordered()
}

func (state *symbolState) lastPrice() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.last
}

func (state *symbolState) change() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.changePct
}

/*
crossLag computes the bar lag at which the anchor's returns most strongly
predict this symbol's returns. Positive lag means the anchor leads. The best
lagged correlation must dominate the contemporaneous baseline by an adaptive
margin, otherwise the co-movement is beta, not lead. Returns (lagBars, corr, ok).
*/
func (state *symbolState) crossLag(anchor *symbolState) (int, float64, bool) {
	anchorSeries := anchor.priceSamples()
	stateSeries := state.priceSamples()

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, 0, false
	}

	interval := config.BarInterval()
	baseline := 0.0

	if corr, ok := numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries); ok {
		baseline = corr
	}

	bestCorr := 0.0
	bestLag := 0

	for lag := 1; lag <= maxLagBars; lag++ {
		shifted := numeric.ShiftPriceSamples(anchorSeries, time.Duration(lag)*interval)
		corr, ok := numeric.HayashiYoshidaCorrelation(shifted, stateSeries)

		if ok && corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	if bestLag <= 0 || bestCorr <= leadlagMinimumLagCorrelation {
		return 0, 0, false
	}

	floor := baseline

	if floor < 0 {
		floor = 0
	}

	margin := leadlagDominanceMarginRel * math.Abs(baseline)

	if margin < leadlagDominanceMarginAbs {
		margin = leadlagDominanceMarginAbs
	}

	if bestCorr <= floor+margin {
		return 0, 0, false
	}

	return bestLag, bestCorr, true
}

/*
contemporaneous returns the unlagged Hayashi-Yoshida correlation against the
anchor when both series have enough overlap.
*/
func (state *symbolState) contemporaneous(anchor *symbolState) (float64, bool) {
	anchorSeries := anchor.priceSamples()
	stateSeries := state.priceSamples()

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, false
	}

	return numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries)
}
