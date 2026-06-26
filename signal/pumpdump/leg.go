package pumpdump

import (
	"math"

	"github.com/theapemachine/symm/statutil"
)

/*
legContext carries the current consolidation-leg price anchor. Session-long
baselines contaminate after leg 1: leg 2 ignition reads "moderate" against a
history that still includes leg 1's pump. The anchor pins precursor to the
CURRENT leg's chop range (legAnchorLow/legAnchorHigh) and is persisted so the
next frame rebuilds leg context from the tree alone — no per-pair store.
*/
type legContext struct {
	anchorLow       float64
	anchorHigh      float64
	exhaustionStamp float64
}

/*
legAnchor derives the current leg's consolidation range. When a prior frame
recorded Faded Exhaustion (lastExhaustionStamp), the anchor is reset to the
chop that followed it, so a new leg starts fresh. The chop is the recent
consolidation window: prices whose lift sits at or below its own median (the
flat range between legs, not the vertical candle). Windows are cadence-derived
from tree stamps via statutil.WindowDepth — no fixed horizon.
*/
func (history measurementHistory) legAnchor(sample tickerSample) legContext {
	context := legContext{exhaustionStamp: history.exhaustionStamp}

	consolidation := history.consolidationPrices(history.exhaustionStamp)

	if len(consolidation) == 0 {
		context.anchorLow = sample.last
		context.anchorHigh = sample.last

		return context
	}

	low := consolidation[0]
	high := consolidation[0]

	for _, price := range consolidation[1:] {
		low = math.Min(low, price)
		high = math.Max(high, price)
	}

	context.anchorLow = low
	context.anchorHigh = high

	return context
}

/*
consolidationPrices returns the recent leg's chop prices after the exhaustion
reset stamp. Samples at or below the lift median are the consolidation; the
vertical-candle samples (high lift) are excluded so the anchor measures the
range price coils in, not the spike it broke from.
*/
func (history measurementHistory) consolidationPrices(afterStamp float64) []float64 {
	if len(history.lasts) == 0 {
		return nil
	}

	liftMedian := statutil.Median(positiveSamples(history.lifts))
	prices := make([]float64, 0, len(history.lasts))

	for index, price := range history.lasts {
		if price <= 0 || history.stamps[index] <= afterStamp {
			continue
		}

		if liftMedian > 0 && history.lifts[index] > liftMedian {
			continue
		}

		prices = append(prices, price)
	}

	if len(prices) == 0 {
		return history.lasts
	}

	return prices
}

/*
anchoredPrecursor scores upward price detachment from the CURRENT leg anchor
high rather than from a session baseline that includes leg 1. The detachment is
normalised by the leg's own range so leg 2 verticality is judged fresh.
*/
func (context legContext) anchoredPrecursor(last float64) float64 {
	if context.anchorHigh <= 0 || last <= context.anchorHigh {
		return 0
	}

	span := context.anchorHigh - context.anchorLow

	if span <= 0 {
		return math.Max(0, math.Log(last/context.anchorHigh))
	}

	return (last - context.anchorHigh) / span
}
