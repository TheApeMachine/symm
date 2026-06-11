package fluid

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	base    *signal.System
	symbols sync.Map
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	system := &System{}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceFluid,
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

/*
Tick runs the shared signal bus loop. Field snapshots publish from market feeds.
*/
func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}

func (system *System) loadSymbol(symbol string) *FluidSymbol {
	if raw, ok := system.symbols.Load(symbol); ok {
		return raw.(*FluidSymbol)
	}

	state, err := NewFluidSymbol(symbol)

	if errnie.Error(err) != nil {
		return nil
	}

	raw, _ := system.symbols.LoadOrStore(symbol, state)

	return raw.(*FluidSymbol)
}
