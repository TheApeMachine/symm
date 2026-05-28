package market

import "math"

/*
WeightedDepthImbalance applies exponential distance decay to book levels so
deep spoof walls contribute less than touch liquidity.
*/
func WeightedDepthImbalance(
	bids, asks []BookLevel,
	mid, decayLambda float64,
) (weightedImbalance float64, ok bool) {
	return WeightedDepthImbalanceFiltered(bids, asks, mid, decayLambda, nil)
}

/*
WeightedDepthImbalanceFiltered is WeightedDepthImbalance with a per-level
exclusion predicate applied before distance decay. A level for which skip
returns true is dropped outright; the survivors are then distance-decayed
exactly as in WeightedDepthImbalance. This is how the toxic-liquidity filter
(§16.4) removes a large near-touch wall that is being pulled as price
approaches — distance decay handles the deep-and-static spoof, the exclusion
handles the near-touch one decay alone cannot. skip may be nil (no exclusion).

The decay kernel is kept exponential (exp(-lambda*d)); it is strictly stronger
than a (1+d)^-2 penalty at the configured lambda, so deep walls already
contribute almost nothing and the toxic exclusion only needs to cover the
near-touch case.
*/
func WeightedDepthImbalanceFiltered(
	bids, asks []BookLevel,
	mid, decayLambda float64,
	skip func(price float64) bool,
) (weightedImbalance float64, ok bool) {
	if mid <= 0 || decayLambda <= 0 {
		return 0, false
	}

	weightedBid := weightedSideVolume(bids, mid, decayLambda, skip)
	weightedAsk := weightedSideVolume(asks, mid, decayLambda, skip)
	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func weightedSideVolume(levels []BookLevel, mid, decayLambda float64, skip func(price float64) bool) float64 {
	weighted := 0.0

	for _, level := range levels {
		if skip != nil && skip(level.Price) {
			continue
		}

		distance := math.Abs(level.Price-mid) / mid
		weight := math.Exp(-decayLambda * distance)
		weighted += level.Volume * weight
	}

	return weighted
}

/*
Level1Imbalance is touch-only bid/ask volume skew.
*/
func Level1Imbalance(bids, asks []BookLevel) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	bid := bids[0].Volume
	ask := asks[0].Volume
	total := bid + ask

	if total <= 0 {
		return 0, false
	}

	return (bid - ask) / total, true
}

/*
FlatDepthImbalance sums volume across all parsed levels without distance decay.
*/
func FlatDepthImbalance(bids, asks []BookLevel) (flatImbalance float64, ok bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.Volume
	}

	for _, level := range asks {
		askVolume += level.Volume
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}

/*
IsSpoofSkew reports deep-book skew that contradicts the touch.
*/
func IsSpoofSkew(weightedImbalance, level1Imbalance, weightedThreshold, level1Reject float64) bool {
	if weightedImbalance > weightedThreshold && level1Imbalance < level1Reject {
		return true
	}

	if weightedImbalance < -weightedThreshold && level1Imbalance > -level1Reject {
		return true
	}

	return false
}
