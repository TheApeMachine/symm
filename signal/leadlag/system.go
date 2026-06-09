package leadlag

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/signal"
)

const (
	priceHistoryCap     = 256
	minLagSamples       = 16
	maxLagBars          = 12
	anchorMoveMinObs    = 12
	anchorMoveAlpha     = 0.05
	anchorMoveMinLogRet = 1e-5
	barInterval         = 5 * time.Minute
	ringSampleSpacing   = 15 * time.Second
)

type System struct {
	base *signal.System
}

var leadLagSection *crossSection

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	leadLagSection = newCrossSection()

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceLeadLag,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity)
		},
	)

	if base == nil {
		return nil
	}

	return &System{base: base}
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

func anchorSymbol() string {
	symbol := viper.GetString("market.anchor_symbol")

	if symbol == "" {
		return "BTC/EUR"
	}

	return symbol
}

type crossSection struct {
	universe       sync.Map
	anchorBaseline moveBaseline
}

type symbolState struct {
	mu           sync.RWMutex
	last         float64
	lastSampleAt time.Time
	prices       numeric.PriceSampleRing
}

func newCrossSection() *crossSection {
	return &crossSection{anchorBaseline: newMoveBaseline()}
}

func (crossSection *crossSection) ensure(symbol string) *symbolState {
	raw, _ := crossSection.universe.LoadOrStore(symbol, &symbolState{
		prices: numeric.NewPriceSampleRing(priceHistoryCap),
	})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

func (crossSection *crossSection) observePrice(symbol string, price float64, at time.Time) {
	if symbol == "" || price <= 0 || at.IsZero() {
		return
	}

	state := crossSection.ensure(symbol)

	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = price

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices.Push(at, price)
}

func (crossSection *crossSection) anchorState() *symbolState {
	return crossSection.ensure(anchorSymbol())
}

func (state *symbolState) priceSamplesInto(destination []numeric.PriceSample) []numeric.PriceSample {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.prices.AppendOrdered(destination)
}

func (state *symbolState) lastPrice() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.last
}

func (state *symbolState) observeTicker(last float64, at time.Time) {
	if last <= 0 || at.IsZero() {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.last = last

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices.Push(at, last)
}
