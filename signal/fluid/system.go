package fluid

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/compute"
)

type System struct {
	base    *signal.System
	symbols sync.Map
	worker  *compute.BatchWorker
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
	system.worker = compute.NewBatchWorker(
		ctx,
		8192,
		signal.ResolveComputeBatchInterval(),
	)

	return system
}

/*
Tick runs the shared signal bus loop. Field snapshots publish from market feeds.
*/
func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	if system.worker != nil {
		system.worker.Close()
	}

	return system.base.Close()
}

func (system *System) enqueue(symbol string, task func(*FluidSymbol)) {
	if system == nil || task == nil {
		return
	}

	state := system.loadSymbol(symbol)

	if state == nil {
		return
	}

	if system.worker == nil {
		task(state)
		return
	}

	system.worker.Submit(func() {
		task(state)
	})
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
