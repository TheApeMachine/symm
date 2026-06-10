package manifold

import (
	"context"
	"time"

	"github.com/spf13/viper"
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
	)

	if base == nil {
		field.Close()
		return nil
	}

	system.base = base

	return system
}

/*
Tick runs the shared signal bus loop and publishes field snapshots on a timer.
*/
func (system *System) Tick() error {
	interval := viper.GetDuration("signals.manifold.snapshot_interval")

	if interval <= 0 {
		interval = 500 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	errCh := make(chan error, 1)

	go func() {
		errCh <- system.base.Tick()
	}()

	for {
		select {
		case err := <-errCh:
			return err
		case at := <-ticker.C:
			if err := system.publishSnapshot(at); errnie.Error(err) != nil {
				continue
			}
		}
	}
}

func (system *System) Close() error {
	system.field.Close()

	return system.base.Close()
}
