package manifold

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	base  *signal.System
	field *Field
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	field, err := newField()

	if errnie.Error(err) != nil {
		return nil
	}

	system := &System{field: field}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourceManifold,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system)
		},
		logic.EntityTrade,
		logic.EntityTick,
		logic.EntityBook,
	)

	if base == nil {
		field.Close()
		return nil
	}

	system.base = base
	system.base.OnSymbols(system.field.RegisterSymbols)
	system.field.SetSnapshotPublisher(system.publishSnapshot)

	return system
}

/*
Tick runs the shared signal bus loop. Field snapshots publish from market feeds.
*/
func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	system.field.Close()

	return system.base.Close()
}
