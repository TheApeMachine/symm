package toxicity

import (
	"context"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	base *signal.System
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceToxicity,
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
