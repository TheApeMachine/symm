package leadlag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/signal"
)

const (
	priceHistoryCap   = 256
	minLagSamples     = 16
	maxLagBars        = 12
	anchorMoveMinObs  = 12
	barInterval       = 5 * time.Minute
	ringSampleSpacing = 15 * time.Second
)

type System struct {
	base         *signal.System
	anchorSymbol string
}

var leadLagSection *crossSection

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	marketConfig, marketErr := config.LoadMarketConfig()
	resolvedAnchor := marketConfig.AnchorSymbol

	if marketErr != nil {
		errnie.Error(fmt.Errorf("leadlag: load market config: %w", marketErr))
	}

	if resolvedAnchor == "" {
		resolvedAnchor = "BTC/EUR"
	}

	leadLagSection = newCrossSection(resolvedAnchor)

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceLeadLag,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity)
		},
		logic.EntityTrade,
		logic.EntityTick,
	)

	if base == nil {
		return nil
	}

	return &System{
		base:         base,
		anchorSymbol: resolvedAnchor,
	}
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

func anchorSymbol() string {
	if leadLagSection == nil {
		return "BTC/EUR"
	}

	return leadLagSection.anchorSymbol
}

type crossSection struct {
	universe       sync.Map
	anchorBaseline moveBaseline
	anchorSymbol   string
}

type symbolState struct {
	last         float64
	lastSampleAt time.Time
	prices       numeric.PriceSampleRing
}

func newCrossSection(anchor ...string) *crossSection {
	resolved := "BTC/EUR"

	if len(anchor) > 0 && anchor[0] != "" {
		resolved = anchor[0]
	}

	return &crossSection{
		anchorSymbol:   resolved,
		anchorBaseline: newMoveBaseline(),
	}
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

func (state *symbolState) observeTicker(price float64, at time.Time) {
	if price <= 0 || at.IsZero() {
		return
	}

	state.last = price

	if !state.lastSampleAt.IsZero() && at.Sub(state.lastSampleAt) < ringSampleSpacing {
		return
	}

	state.lastSampleAt = at
	state.prices.Push(at, price)
}

func (state *symbolState) priceSamplesInto(destination []numeric.PriceSample) []numeric.PriceSample {
	return state.prices.AppendOrdered(destination)
}

func (state *symbolState) lastPrice() float64 {
	return state.last
}
