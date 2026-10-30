package pumpdump

import (
	"context"
	"math"
	"slices"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/volumeclock"
)

// midReturnDeadband suppresses coiled_compression when the bar already repriced.
const midReturnDeadband = 5e-4

// peakReturnFloor is the minimum recent up-bar needed before a dip counts as blowoff.
const peakReturnFloor = 5 * midReturnDeadband

type causalEstimator struct {
	count float64
	mean  float64
	m2    float64
}

func (ce *causalEstimator) Step(value float64) (hasBaseline bool, baseline float64) {
	priorCount := ce.count
	priorMean := ce.mean

	ce.count++
	delta := value - ce.mean
	ce.mean += delta / ce.count
	ce.m2 += delta * (value - ce.mean)

	if priorCount == 0 {
		return false, 0
	}

	return true, priorMean
}

type symbolState struct {
	clock                 volumeclock.Clock
	volumeRateEstimator   causalEstimator
	tradeDensityEstimator causalEstimator
	barDurationEstimator  causalEstimator
	recentPeakPos         float64
	lastStarved           float64
	lastCoiled            float64
	lastRejection         float64
	lastBlowoff           float64
}

type Signal struct {
	*runtime.System
	states *sync.Map
	keys   []string
	books  *broker.Book
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: &sync.Map{},
		keys: []string{
			"coiled_compression",
			"rejection_tick",
			"starved_tape",
			"blowoff_dip",
		},
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)
	return signal
}

func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	if err := parent.RunTrades(signal.Context(), trades, func(trade kraken.TradeData) {
		signal.step(value, trade)
	}); err != nil {
		signal.Error(err)
	}

	signal.appendBoardStarvation(value)

	return value
}

/*
appendBoardStarvation publishes the median last starved_tape across symbols
as a regime oracle (flat / dead board), not a single-name reading.
*/
func (signal *Signal) appendBoardStarvation(value *data.Value) {
	if signal.Status() != runtime.READY || value == nil {
		return
	}

	var scores []float64
	signal.states.Range(func(_, untyped any) bool {
		scores = append(scores, untyped.(*symbolState).lastStarved)
		return true
	})

	if len(scores) == 0 {
		return
	}

	slices.Sort(scores)
	median := scores[len(scores)/2]
	if len(scores)%2 == 0 {
		median = (scores[len(scores)/2-1] + scores[len(scores)/2]) / 2
	}

	value.Append([]float64{median}, append([]string{
		signal.Name(),
		"aggregate",
	}, "board_starvation")...)
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || !parent.HasFrame(value) {
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty
	if price <= 0 || qty <= 0 {
		return value
	}

	var bookAvailable bool
	var bid, ask float64
	var hasTouch bool

	signal.books.Book(trade.Symbol, func(b *spotbook.Book) {
		bookAvailable = true

		bestBid := b.BestBid()
		bestAsk := b.BestAsk()
		if bestBid != nil && bestAsk != nil && bestBid.Price != nil && bestAsk.Price != nil {
			bid = bestBid.Price.Float64()
			ask = bestAsk.Price.Float64()
			hasTouch = true
		}
	})

	if !bookAvailable || !hasTouch {
		return value
	}

	atNanos := float64(value.Timestamp)
	midpoint := (bid + ask) * 0.5

	untypedState, _ := signal.states.LoadOrStore(trade.Symbol, &symbolState{})
	state := untypedState.(*symbolState)

	tick := state.clock.Step(price, qty, atNanos, midpoint)
	bar, closed := tick.Bar, tick.Closed

	var coiled, rejection, starved, blowoffDip float64
	if closed {
		var volumeRate, volRatio, posReturn, negReturn float64

		if bar.Duration > 0 && !math.IsInf(bar.Duration, 0) {
			volumeRate = bar.Quantity / bar.Duration
			hasVolBase, volBase := state.volumeRateEstimator.Step(volumeRate)
			if hasVolBase && volBase > 0 && math.Abs(volBase) >= 1e-6*math.Abs(volumeRate) {
				volRatio = volumeRate / volBase
			}

			tradeDensity := bar.Trades / bar.Duration
			if hasBase, base := state.tradeDensityEstimator.Step(tradeDensity); hasBase && base > 0 {
				starved = math.Min(1, base/(tradeDensity+1e-12))
			}

			durationStress := 0.0
			if hasDur, baseDur := state.barDurationEstimator.Step(bar.Duration); hasDur && baseDur > 0 {
				durationStress = math.Min(1, bar.Duration/(baseDur+1e-12))
			}

			midMove := 0.0
			if bar.FromMid > 0 && bar.AtMid > 0 {
				midMove = math.Abs(math.Log(bar.AtMid / bar.FromMid))
			}
			flat := 1.0 - math.Min(1.0, midMove/midReturnDeadband)
			starved = math.Max(starved, flat*durationStress)
		}

		if bar.FromMid > 0 && bar.AtMid > 0 {
			midReturn := math.Log(bar.AtMid / bar.FromMid)
			posReturn = math.Max(midReturn, 0)
			negReturn = math.Max(-midReturn, 0)
		}

		quiet := 1.0 - math.Min(1.0, posReturn/midReturnDeadband)
		if volRatio > 0 {
			coiled = volRatio * quiet
		}

		rejection = negReturn

		if negReturn >= midReturnDeadband &&
			state.recentPeakPos >= peakReturnFloor &&
			state.recentPeakPos > 0 {
			blowoffDip = math.Min(1, negReturn/state.recentPeakPos)
		}

		if posReturn >= state.recentPeakPos {
			state.recentPeakPos = posReturn
		} else if state.recentPeakPos > 0 {
			state.recentPeakPos *= 0.92
		}

		state.lastStarved = starved
		state.lastCoiled = coiled
		state.lastRejection = rejection
		state.lastBlowoff = blowoffDip
	} else {
		coiled = state.lastCoiled
		rejection = state.lastRejection
		starved = state.lastStarved
		blowoffDip = state.lastBlowoff
	}

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	out := value.Append([]float64{
		coiled,
		rejection,
		starved,
		blowoffDip,
	}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal":     signal.Name(),
		"trade":      trade,
		"coiled":     coiled,
		"rejection":  rejection,
		"starved":    starved,
		"blowoffDip": blowoffDip,
	})

	return out
}
