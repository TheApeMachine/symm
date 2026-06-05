package depthflow

import (
	"math"
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/toxicity"
)

func (state *DepthSymbol) level1ImbalanceLocked(bids, asks []market.BookLevel) (float64, bool) {
	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	total := bids[0].Qty + asks[0].Qty

	if total <= 0 {
		return 0, false
	}

	return (bids[0].Qty - asks[0].Qty) / total, true
}

func (state *DepthSymbol) flatImbalanceLocked(bids, asks []market.BookLevel) (float64, bool) {
	bidVolume := 0.0
	askVolume := 0.0

	for _, level := range bids {
		bidVolume += level.Qty
	}

	for _, level := range asks {
		askVolume += level.Qty
	}

	total := bidVolume + askVolume

	if total <= 0 {
		return 0, false
	}

	return (bidVolume - askVolume) / total, true
}

func (state *DepthSymbol) weightedImbalanceLocked(
	bids, asks []market.BookLevel, mid float64,
) (float64, bool) {
	if mid <= 0 {
		return 0, false
	}

	if len(bids) == 0 || len(asks) == 0 {
		return 0, false
	}

	spread := asks[0].Price - bids[0].Price

	if spread <= 0 {
		return 0, false
	}

	weightedBid := 0.0
	weightedAsk := 0.0

	for _, level := range bids {
		if state.isToxicLevelLocked(level.Price) {
			continue
		}

		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedBid += level.Qty * weight
	}

	for _, level := range asks {
		if state.isToxicLevelLocked(level.Price) {
			continue
		}

		weight := math.Exp(-math.Abs(level.Price-mid) / spread)
		weightedAsk += level.Qty * weight
	}

	total := weightedBid + weightedAsk

	if total <= 0 {
		return 0, false
	}

	return (weightedBid - weightedAsk) / total, true
}

func (state *DepthSymbol) isToxicLevelLocked(price float64) bool {
	return toxicity.IsToxic(state.symbol, price, time.Now())
}

func (state *DepthSymbol) isSpoofSkewLocked(weighted, level1 float64) bool {
	weightedThreshold := state.spoofWeightedThreshold
	level1Reject := state.spoofLevel1Reject

	if math.Abs(weighted) < weightedThreshold {
		return false
	}

	if weighted > 0 && level1 < level1Reject {
		return true
	}

	if weighted < 0 && level1 > -level1Reject {
		return true
	}

	return false
}
