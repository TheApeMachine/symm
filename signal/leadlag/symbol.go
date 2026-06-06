package leadlag

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
)

const (
	priceHistoryCap              = 256
	maxLagBars                   = 12
	minLagSamples                = 16
	leadlagDominanceMarginAbs    = 0.1
	leadlagDominanceMarginRel    = 0.15
	leadlagMinimumLagCorrelation = 0.1
	barInterval                  = 5 * time.Minute
)

/*
symbolState tracks the rolling price path needed to compute a real
cross-correlation against the anchor pair. The ring records (timestamp, price)
pairs sampled on every ticker frame, so the lag can be measured in bars rather
than in a same-instant cross-section spread.
*/
type symbolState struct {
	mu      sync.RWMutex
	last    float64
	prices  numeric.PriceSampleRing
	tracked *types.Category
}

func newSymbolState() *symbolState {
	return &symbolState{
		prices:  numeric.NewPriceSampleRing(priceHistoryCap),
		tracked: types.NewCategory(types.CategoryTypeNone),
	}
}

func (state *symbolState) observeTicker(last float64, at time.Time) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = last
	state.prices.Push(at, last)
}

func (state *symbolState) priceSamples() []numeric.PriceSample {
	return state.priceSamplesInto(nil)
}

func (state *symbolState) priceSamplesInto(
	destination []numeric.PriceSample,
) []numeric.PriceSample {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.prices.AppendOrdered(destination)
}

func (state *symbolState) lastPrice() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.last
}

/*
crossLag computes the bar lag at which the anchor's returns most strongly
predict this symbol's returns. Positive lag means the anchor leads. The best
lagged correlation must dominate the contemporaneous baseline by an adaptive
margin, otherwise the co-movement is beta, not lead. Returns (lagBars, corr, ok).
*/
func (state *symbolState) crossLag(anchor *symbolState) (int, float64, bool) {
	var anchorBuffer [priceHistoryCap]numeric.PriceSample
	var stateBuffer [priceHistoryCap]numeric.PriceSample
	var shiftBuffer [priceHistoryCap]numeric.PriceSample

	anchorSeries := anchor.priceSamplesInto(anchorBuffer[:0])
	stateSeries := state.priceSamplesInto(stateBuffer[:0])

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, 0, false
	}

	interval := barInterval
	baseline := 0.0

	if corr, ok := numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries); ok {
		baseline = corr
	}

	bestCorr := 0.0
	bestLag := 0

	for lag := 1; lag <= maxLagBars; lag++ {
		shifted := numeric.ShiftPriceSamplesInto(
			shiftBuffer[:0], anchorSeries, time.Duration(lag)*interval,
		)
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
	var anchorBuffer [priceHistoryCap]numeric.PriceSample
	var stateBuffer [priceHistoryCap]numeric.PriceSample

	anchorSeries := anchor.priceSamplesInto(anchorBuffer[:0])
	stateSeries := state.priceSamplesInto(stateBuffer[:0])

	if len(anchorSeries) < minLagSamples || len(stateSeries) < minLagSamples {
		return 0, false
	}

	return numeric.HayashiYoshidaCorrelation(anchorSeries, stateSeries)
}
