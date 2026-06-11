package toxicity

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/symm/numeric"
)

const dynamicsHistoryCap = 64

func (state *symbolState) recordLevelSizeFrac(qty, sideDepth float64) {
	if qty <= 0 || sideDepth <= 0 {
		return
	}

	state.levelSizeFracs = appendRingFloat(state.levelSizeFracs, qty/sideDepth, dynamicsHistoryCap)
}

func (state *symbolState) recordChurnRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.churnRatios = appendRingFloat(state.churnRatios, ratio, dynamicsHistoryCap)
}

func (state *symbolState) recordFillMatchRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.fillMatchRatios = appendRingFloat(state.fillMatchRatios, ratio, dynamicsHistoryCap)
}

func (state *symbolState) observeSpreadPct(price float64) {
	if state.mid <= 0 || price <= 0 {
		return
	}

	spreadPct := math.Abs(price-state.mid) / state.mid

	if spreadPct <= 0 {
		return
	}

	if state.spreadPctEMA <= 0 {
		state.spreadPctEMA = spreadPct
		return
	}

	alpha := state.flowSmoothingAlpha(time.Time{})

	state.spreadPctEMA = (1-alpha)*state.spreadPctEMA + alpha*spreadPct
}

func (state *symbolState) priceMatchTolerance(price float64) float64 {
	if price <= 0 {
		return math.Inf(1)
	}

	tickSize, tickErr := state.pair.TickSizeFloat()

	if tickErr == nil && tickSize > 0 {
		return tickSize / price
	}

	if state.spreadPctEMA > 0 {
		return state.spreadPctEMA
	}

	return numeric.MedianAbsolute(state.tradePriceDeviations(price))
}

func (state *symbolState) tradePriceDeviations(reference float64) []float64 {
	deviations := make([]float64, 0, len(state.trades))

	for _, trade := range state.trades {
		if trade.price <= 0 {
			continue
		}

		deviations = append(deviations, math.Abs(trade.price-reference)/reference)
	}

	return deviations
}

func (state *symbolState) touchProximityPct() float64 {
	tickSize, tickErr := state.pair.TickSizeFloat()

	if state.mid > 0 && tickErr == nil && tickSize > 0 {
		return tickSize / state.mid
	}

	if state.spreadPctEMA > 0 {
		return state.spreadPctEMA
	}

	if state.mid > 0 {
		deviations := state.tradePriceDeviations(state.mid)

		if median := numeric.MedianAbsolute(deviations); median > 0 {
			return median
		}
	}

	return math.Inf(1)
}

func (state *symbolState) recordCancelQty(qty float64) {
	if qty <= 0 {
		return
	}

	state.cancelQtys = appendRingFloat(state.cancelQtys, qty, dynamicsHistoryCap)
}

func (state *symbolState) largeBlockQtyThreshold(sideDepth float64) float64 {
	if sideDepth <= 0 {
		return math.Inf(1)
	}

	if len(state.cancelQtys) >= 3 {
		sorted := numeric.CopySorted(state.cancelQtys)

		return numeric.PercentileSorted(sorted, 0.5)
	}

	if len(state.levelSizeFracs) >= 3 {
		sorted := numeric.CopySorted(state.levelSizeFracs)
		frac := numeric.PercentileSorted(sorted, 0.75)

		return frac * sideDepth
	}

	medianQty := medianLevelQty(state.levels)

	if medianQty > 0 {
		return medianQty
	}

	return sideDepth / math.Max(1, math.Sqrt(sideDepth))
}

func medianLevelQty(levels map[l2Key]*l2Level) float64 {
	if len(levels) == 0 {
		return 0
	}

	quantities := make([]float64, 0, len(levels))

	for _, level := range levels {
		if level == nil || level.qty <= 0 {
			continue
		}

		quantities = append(quantities, level.qty)
	}

	if len(quantities) == 0 {
		return 0
	}

	sort.Float64s(quantities)

	middle := len(quantities) / 2

	if len(quantities)%2 == 0 {
		return (quantities[middle-1] + quantities[middle]) / 2
	}

	return quantities[middle]
}

func (state *symbolState) churnRatioGate() float64 {
	if len(state.churnRatios) >= 3 {
		sorted := numeric.CopySorted(state.churnRatios)

		return numeric.PercentileSorted(sorted, 0.75)
	}

	return 1
}

func (state *symbolState) fillCoverageGate() float64 {
	if len(state.fillMatchRatios) >= 3 {
		sorted := numeric.CopySorted(state.fillMatchRatios)

		return numeric.PercentileSorted(sorted, 0.5)
	}

	return 1
}

func (state *symbolState) recordFillCoverage(matched, qty float64) {
	if qty <= 0 {
		return
	}

	state.recordFillMatchRatio(matched / qty)
}

func (state *symbolState) flowSmoothingAlpha(_ time.Time) float64 {
	if len(state.trades) < 2 {
		return 0.05
	}

	span := state.trades[len(state.trades)-1].at.Sub(state.trades[0].at)

	if span <= 0 {
		return 0.05
	}

	meanInterval := span / time.Duration(len(state.trades)-1)
	window := 30 * time.Second

	if meanInterval <= 0 {
		return 0.05
	}

	alpha := float64(window) / float64(meanInterval+window)

	return math.Min(math.Max(alpha, 0.01), 0.5)
}

func (state *symbolState) recordVacuumRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.vacuumRatios = appendRingFloat(state.vacuumRatios, ratio, dynamicsHistoryCap)
}

func (state *symbolState) vacuumStrengthLimit(threshold float64) float64 {
	if threshold <= 0 {
		return 1
	}

	if len(state.vacuumRatios) >= 3 {
		sorted := numeric.CopySorted(state.vacuumRatios)
		peak := numeric.PercentileSorted(sorted, 0.9)

		return math.Max(peak/threshold, peak)
	}

	if state.peakVacuumRatio > 0 {
		return state.peakVacuumRatio / threshold
	}

	return 1
}

func appendRingFloat(values []float64, value float64, capacity int) []float64 {
	values = append(values, value)

	if len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}

func (state *symbolState) supportRatioGate(threshold float64) float64 {
	if threshold <= 0 {
		return 0
	}

	if len(state.vacuumRatios) < 3 {
		return 0
	}

	sorted := numeric.CopySorted(state.vacuumRatios)
	low := numeric.PercentileSorted(sorted, 0.25)

	return low / threshold
}
