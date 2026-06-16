package toxicity

import (
	"math"
	"sort"
	"time"
)

func (state *symbolState) recordTradeInterval(at time.Time) {
	if !state.hasLastTradeAt {
		state.lastTradeAt = at
		state.hasLastTradeAt = true

		return
	}

	interval := at.Sub(state.lastTradeAt).Seconds()

	if interval > 0 {
		state.tradeIntervals = appendRingFloat(state, state.tradeIntervals, interval)
	}

	state.lastTradeAt = at
}

func (state *symbolState) recordLevelLifetime(age time.Duration) {
	if age <= 0 {
		return
	}

	state.levelLifetimes = appendRingFloat(
		state,
		state.levelLifetimes,
		age.Seconds(),
	)
}

func (state *symbolState) recordBookPulse(now time.Time) {
	if state.lastBookPulse.IsZero() {
		state.lastBookPulse = now

		return
	}

	interval := now.Sub(state.lastBookPulse).Seconds()

	if interval > 0 {
		state.bookPulseIntervals = appendRingFloat(state, state.bookPulseIntervals, interval)
	}

	state.lastBookPulse = now
}

func (state *symbolState) recordChurnDuration(duration time.Duration) {
	if duration <= 0 {
		return
	}

	state.churnDurations = appendRingFloat(state, state.churnDurations, duration.Seconds())
}

func (state *symbolState) recordPriceObservation(price float64) {
	if price <= 0 {
		return
	}

	for key := range state.levels {
		step := math.Abs(price - key.price)

		if step <= 0 {
			continue
		}

		state.priceIncrements = appendRingFloat(state, state.priceIncrements, step)
	}

	for _, trade := range state.trades {
		step := math.Abs(price - trade.price)

		if step <= 0 {
			continue
		}

		state.priceIncrements = appendRingFloat(state, state.priceIncrements, step)
	}
}

func (state *symbolState) trimTrades(now time.Time) {
	window := state.tradeMatchWindow()

	if window <= 0 {
		capacity := state.tradeRetentionCount()

		if len(state.trades) <= capacity {
			return
		}

		state.trades = state.trades[len(state.trades)-capacity:]

		return
	}

	cutoff := now.Add(-window)
	trimIndex := 0

	for trimIndex < len(state.trades) && state.trades[trimIndex].at.Before(cutoff) {
		trimIndex++
	}

	if trimIndex > 0 {
		state.trades = state.trades[trimIndex:]
	}
}

func (state *symbolState) tradeMatchWindow() time.Duration {
	if len(state.trades) >= 2 {
		span := state.trades[len(state.trades)-1].at.Sub(state.trades[0].at)

		if span > 0 {
			return span
		}
	}

	if len(state.tradeIntervals) >= 3 {
		median := sampleMedian(state.tradeIntervals)
		p75 := sampleQuantile(0.75, state.tradeIntervals)

		return time.Duration((median + p75) * float64(time.Second))
	}

	if len(state.bookPulseIntervals) >= 3 {
		median := sampleMedian(state.bookPulseIntervals)

		return time.Duration(median * float64(time.Second))
	}

	return 0
}

func (state *symbolState) toxicMaxAge() time.Duration {
	if len(state.levelLifetimes) >= 3 {
		p75 := sampleQuantile(0.75, state.levelLifetimes)

		return time.Duration(p75 * float64(time.Second))
	}

	if len(state.bookPulseIntervals) >= 3 {
		median := sampleMedian(state.bookPulseIntervals)

		return time.Duration(median * float64(time.Second))
	}

	return 0
}

func (state *symbolState) toxicCooldown() time.Duration {
	maxAge := state.toxicMaxAge()
	matchWindow := state.tradeMatchWindow()

	if maxAge > 0 && matchWindow > 0 {
		return maxAge + matchWindow
	}

	if maxAge > 0 {
		return maxAge
	}

	if matchWindow > 0 {
		return matchWindow
	}

	if len(state.churnDurations) >= 3 {
		p75 := sampleQuantile(0.75, state.churnDurations)

		return time.Duration(p75 * float64(time.Second))
	}

	return 0
}

func (state *symbolState) flashChurnWindow() time.Duration {
	if len(state.churnDurations) >= 3 {
		p75 := sampleQuantile(0.75, state.churnDurations)

		return time.Duration(p75 * float64(time.Second))
	}

	if len(state.bookPulseIntervals) >= 3 {
		median := sampleMedian(state.bookPulseIntervals)

		return time.Duration(median * float64(time.Second))
	}

	return 0
}

func (state *symbolState) tradeRetentionCount() int {
	if len(state.tradeIntervals) >= 3 {
		span := intervalSpanSeconds(state.tradeIntervals)
		median := sampleMedian(state.tradeIntervals)

		if median > 0 && span > 0 {
			return int(math.Ceil(span/median)) + 1
		}
	}

	if len(state.trades) == 0 {
		return 1
	}

	return len(state.trades) + 1
}

func (state *symbolState) priceKeyScale() float64 {
	if len(state.priceIncrements) >= 3 {
		step := sampleMedianAbsolute(state.priceIncrements)

		if step > 0 {
			return 1 / step
		}
	}

	if state.mid > 0 {
		proximity := state.touchProximityPct()

		if proximity > 0 && proximity < math.Inf(1) {
			step := state.mid * proximity

			if step > 0 {
				return 1 / step
			}
		}
	}

	return 0
}

func (state *symbolState) recordLevelSizeFrac(qty, sideDepth float64) {
	if qty <= 0 || sideDepth <= 0 {
		return
	}

	state.levelSizeFracs = appendRingFloat(state, state.levelSizeFracs, qty/sideDepth)
}

func (state *symbolState) recordChurnRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.churnRatios = appendRingFloat(state, state.churnRatios, ratio)
}

func (state *symbolState) recordFillMatchRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.fillMatchRatios = appendRingFloat(state, state.fillMatchRatios, ratio)
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

	alpha := state.flowSmoothingAlpha()

	if alpha <= 0 {
		state.spreadPctEMA = spreadPct
		return
	}

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

	return sampleMedianAbsolute(state.tradePriceDeviations(price))
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

		if median := sampleMedianAbsolute(deviations); median > 0 {
			return median
		}
	}

	return math.Inf(1)
}

func (state *symbolState) recordCancelQty(qty float64) {
	if qty <= 0 {
		return
	}

	state.cancelQtys = appendRingFloat(state, state.cancelQtys, qty)
}

func (state *symbolState) largeBlockQtyThreshold(sideDepth float64) float64 {
	if sideDepth <= 0 {
		return math.Inf(1)
	}

	if len(state.cancelQtys) >= 3 {
		return sampleQuantile(0.5, state.cancelQtys)
	}

	if len(state.levelSizeFracs) >= 3 {
		frac := sampleQuantile(0.75, state.levelSizeFracs)

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

	return sampleMedian(quantities)
}

func (state *symbolState) churnRatioGate() float64 {
	if len(state.churnRatios) >= 3 {
		return sampleQuantile(0.75, state.churnRatios)
	}

	return 0
}

func (state *symbolState) fillCoverageGate() float64 {
	if len(state.fillMatchRatios) >= 3 {
		return sampleQuantile(0.5, state.fillMatchRatios)
	}

	return 1
}

func (state *symbolState) recordFillCoverage(matched, qty float64) {
	if qty <= 0 {
		return
	}

	state.recordFillMatchRatio(matched / qty)
}

func (state *symbolState) flowSmoothingWindow() time.Duration {
	matchWindow := state.tradeMatchWindow()
	maxAge := state.toxicMaxAge()

	if matchWindow > 0 && maxAge > 0 {
		return matchWindow + maxAge
	}

	if matchWindow > 0 {
		return matchWindow
	}

	if maxAge > 0 {
		return maxAge
	}

	if len(state.trades) >= 2 {
		return state.trades[len(state.trades)-1].at.Sub(state.trades[0].at)
	}

	if len(state.bookPulseIntervals) >= 1 {
		median := sampleMedian(state.bookPulseIntervals)

		return time.Duration(median * float64(time.Second))
	}

	return 0
}

func (state *symbolState) meanTradeIntervalSeconds() float64 {
	if len(state.tradeIntervals) >= 1 {
		return sampleMedian(state.tradeIntervals)
	}

	if len(state.trades) >= 2 {
		span := state.trades[len(state.trades)-1].at.Sub(state.trades[0].at).Seconds()
		count := float64(len(state.trades) - 1)

		if count > 0 && span > 0 {
			return span / count
		}
	}

	if len(state.bookPulseIntervals) >= 1 {
		return sampleMedian(state.bookPulseIntervals)
	}

	return 0
}

func (state *symbolState) flowSmoothingAlpha() float64 {
	meanInterval := state.meanTradeIntervalSeconds()
	window := state.flowSmoothingWindow()

	if meanInterval <= 0 || window <= 0 {
		return 0
	}

	windowSeconds := float64(window) / float64(time.Second)

	return windowSeconds / (meanInterval + windowSeconds)
}

func (state *symbolState) recordVacuumRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	state.vacuumRatios = appendRingFloat(state, state.vacuumRatios, ratio)
}

func (state *symbolState) vacuumStrengthLimit(threshold float64) float64 {
	if threshold <= 0 {
		return 1
	}

	if len(state.vacuumRatios) >= 3 {
		peak := sampleQuantile(0.9, state.vacuumRatios)

		return math.Max(peak/threshold, peak)
	}

	if state.peakVacuumRatio > 0 {
		return state.peakVacuumRatio / threshold
	}

	return 1
}

func appendRingFloat(state *symbolState, values []float64, value float64) []float64 {
	values = append(values, value)
	capacity := state.observationHistoryCapacity(values)

	if capacity <= 0 || len(values) <= capacity {
		return values
	}

	return values[len(values)-capacity:]
}

func (state *symbolState) observationHistoryCapacity(values []float64) int {
	if len(values) == 0 {
		return 1
	}

	if len(values) < 3 {
		return len(values) + 1
	}

	span := intervalSpanSeconds(values)

	if span <= 0 {
		return len(values) + 1
	}

	capacity := int(span) + 1

	if capacity < len(values) {
		return len(values)
	}

	return capacity
}

func intervalSpanSeconds(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return sorted[len(sorted)-1] - sorted[0]
}

func (state *symbolState) supportRatioGate(threshold float64) float64 {
	if threshold <= 0 {
		return 0
	}

	if len(state.vacuumRatios) < 3 {
		return 0
	}

	low := sampleQuantile(0.25, state.vacuumRatios)

	return low / threshold
}
