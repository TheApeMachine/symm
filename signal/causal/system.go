package causal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

var (
	sectionLoader market.CrossSectionOnce
	crossSection  *market.CrossSection
)

type System struct {
	base               *signal.System
	symbols            sync.Map
	contagionEstimator *correlation.Contagion
	lastPublish        time.Time
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	var err error

	crossSection, err = market.LoadCrossSection(&sectionLoader)

	if errnie.Error(err) != nil {
		return nil
	}

	system := &System{}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceCausal,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system)
		},
		logic.EntityTrade,
		logic.EntityTick,
		logic.EntityBook,
	)

	if base == nil {
		return nil
	}

	system.base = base

	return system
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

func (system *System) loadSymbol(symbol string) (*CausalSymbol, error) {
	raw, loaded := system.symbols.Load(symbol)

	if loaded {
		state, ok := raw.(*CausalSymbol)

		if !ok {
			return nil, errnie.Error(fmt.Errorf(
				"causal: symbol %q has unexpected state type %T",
				symbol,
				raw,
			))
		}

		return state, nil
	}

	state, err := NewCausalSymbol()

	if errnie.Error(err) != nil {
		return nil, err
	}

	actual, _ := system.symbols.LoadOrStore(symbol, state)
	loadedState, ok := actual.(*CausalSymbol)

	if !ok {
		return nil, errnie.Error(fmt.Errorf(
			"causal: symbol %q has unexpected state type %T",
			symbol,
			actual,
		))
	}

	return loadedState, nil
}

func (system *System) shouldPublish(now time.Time) bool {
	interval := viper.GetDuration("signals.causal.publish_interval")

	if interval <= 0 {
		return false
	}

	if now.Sub(system.lastPublish) < interval {
		return false
	}

	system.lastPublish = now

	return true
}
