package hawkes

import (
	"context"
	"sync"

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
		logic.SourceHawkes,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system)
		},
	)

	if base == nil {
		return nil
	}

	system.base = base

	return system
}

func (system *System) loadSymbol(symbol string) *HawkesSymbol {
	raw, _ := system.symbols.LoadOrStore(symbol, NewHawkesSymbol())

	state, ok := raw.(*HawkesSymbol)

	if !ok {
		return nil
	}

	return state
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}
