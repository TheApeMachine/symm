package pumpdump

import (
	"context"
	"math"
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

// starvedRegimeNeff is the EMA window for chronic thin-tape posture on starved_tape.
const starvedRegimeNeff = 64.0

// minCompletedVolumeBars before starved_tape publishes non-zero opposition evidence.
const minCompletedVolumeBars = 1.0

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
	starvedRegime         float64
	lastCoiled            float64
	lastRejection         float64
	lastBlowoff           float64
	prevBarDuration       float64
	lastClockAcceleration float64
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
			"clock_acceleration",
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

	signal.appendBoardCoiled(value)

	return value
}

/*
appendBoardCoiled publishes the median last coiled_compression across symbols
as the enter-confidence oracle (R09): board-wide quiet coiling before ignition.
*/
func (signal *Signal) appendBoardCoiled(value *data.Value) {
	if signal.Status() != runtime.READY || value == nil {
		return
	}

	var scores []float64
	signal.states.Range(func(_, untyped any) bool {
		scores = append(scores, untyped.(*symbolState).lastCoiled)
		return true
	})

	median, ok := parent.Median(scores)
	if !ok {
		return
	}

	value.Append([]float64{median}, append([]string{
		signal.Name(),
		"aggregate",
	}, "board_coiled")...)
}

func publishedStarved(state *symbolState, instant float64) float64 {
	starved := math.Max(instant, state.starvedRegime)
	if state.clock.CompletedBars < minCompletedVolumeBars {
		return 0
	}
	return starved
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY {
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

	var coiled, rejection, starved, blowoffDip, clockAccel float64
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

		if bar.Duration > 0 && state.prevBarDuration > 0 {
			clockAccel = (state.prevBarDuration - bar.Duration) / state.prevBarDuration
		}
		state.prevBarDuration = bar.Duration
		state.lastClockAcceleration = clockAccel

		if state.starvedRegime == 0 {
			state.starvedRegime = starved
		} else {
			alpha := 2.0 / (starvedRegimeNeff + 1.0)
			state.starvedRegime = (1-alpha)*state.starvedRegime + alpha*starved
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
		clockAccel = state.lastClockAcceleration
	}

	starved = publishedStarved(state, starved)

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	out := value.Append([]float64{
		coiled,
		rejection,
		starved,
		blowoffDip,
		clockAccel,
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
