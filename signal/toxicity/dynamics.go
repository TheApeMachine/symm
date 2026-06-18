package toxicity

import (
	"math"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/statistic"
)

func newSymbolState(pair Pair) *symbolState {
	return &symbolState{
		pair:            pair,
		timing:          adaptive.NewTimedContext(),
		gates:           algorithm.NewBookGates(),
		toxic:           algorithm.NewEvidenceRegistry(),
		priceIncrements: statistic.NewObservationRing(),
		orders:          make(map[string]*orderState),
		levels:          make(map[l2Key]*l2Level),
		churn:           make(map[l2Key]*levelChurnWindow),
	}
}

func (state *symbolState) recordEventAt(eventAt time.Time) {
	if eventAt.IsZero() {
		return
	}

	if eventAt.After(state.lastEventAt) {
		state.lastEventAt = eventAt
	}
}

func (state *symbolState) tradeSpan() time.Duration {
	if len(state.trades) < 2 {
		return 0
	}

	span := state.trades[len(state.trades)-1].at.Sub(state.trades[0].at)

	if span <= 0 {
		return 0
	}

	return span
}

func (state *symbolState) tradeRetentionCount() int {
	if len(state.trades) >= 2 {
		count := state.timing.TradeRetentionCount()

		if count > len(state.trades) {
			return count
		}
	}

	if len(state.trades) == 0 {
		return 1
	}

	return len(state.trades) + 1
}

func (state *symbolState) recordTradeInterval(at time.Time) {
	if !state.hasLastTradeAt {
		state.lastTradeAt = at
		state.hasLastTradeAt = true

		return
	}

	interval := at.Sub(state.lastTradeAt).Seconds()

	if interval > 0 {
		state.timing.TradeIntervals.Observe(interval)
	}

	state.lastTradeAt = at
}

func (state *symbolState) recordLevelLifetime(age time.Duration) {
	if age <= 0 {
		return
	}

	state.timing.LevelLifetimes.Observe(age.Seconds())
}

func (state *symbolState) recordBookPulse(now time.Time) {
	if state.lastBookPulse.IsZero() {
		state.lastBookPulse = now

		return
	}

	interval := now.Sub(state.lastBookPulse).Seconds()

	if interval > 0 {
		state.timing.BookPulseIntervals.Observe(interval)
	}

	state.lastBookPulse = now
}

func (state *symbolState) recordChurnDuration(duration time.Duration) {
	if duration <= 0 {
		return
	}

	state.timing.ChurnDurations.Observe(duration.Seconds())
}

func (state *symbolState) recordPriceObservation(price float64) {
	if price <= 0 {
		return
	}

	levelKeys := make([]l2Key, 0, len(state.levels))

	for key := range state.levels {
		levelKeys = append(levelKeys, key)
	}

	for _, key := range levelKeys {
		step := math.Abs(price - key.price)

		if step <= 0 {
			continue
		}

		state.priceIncrements.Observe(step)
	}

	for _, trade := range state.trades {
		step := math.Abs(price - trade.price)

		if step <= 0 {
			continue
		}

		state.priceIncrements.Observe(step)
	}
}

func (state *symbolState) trimTrades(now time.Time) {
	window := state.timing.MatchWindow(state.tradeSpan())

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

func clampRoundedInt64(value float64) int64 {
	rounded := math.Round(value)

	switch {
	case rounded > float64(math.MaxInt64):
		return math.MaxInt64
	case rounded < float64(math.MinInt64):
		return math.MinInt64
	default:
		return int64(rounded)
	}
}

func observeLevelSizeFraction(state *symbolState, side byte, qty float64) {
	if qty <= 0 {
		return
	}

	if depth := state.flow.SideDepth(side); depth > 0 {
		state.gates.LevelSizeFracs.Observe(qty / depth)
	}
}

func (state *symbolState) priceKeyScale() float64 {
	if state.priceIncrements.Len() >= 3 {
		step := state.priceIncrements.MedianAbsolute()

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

	alpha := state.timing.FlowSmoothingAlpha(
		state.timing.MatchWindow(state.tradeSpan()),
		state.tradeSpan(),
		len(state.trades),
	)

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

	return statistic.MedianAbsoluteOf(state.tradePriceDeviations(price))
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

		if median := statistic.MedianAbsoluteOf(deviations); median > 0 {
			return median
		}
	}

	return math.Inf(1)
}
