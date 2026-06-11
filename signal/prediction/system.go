package prediction

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	base  *signal.System
	chart *Chart
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	system := &System{}

	base := signal.NewSystem(
		ctx,
		pool,
		logic.SourcePrediction,
		func(symbol string, entity *logic.Entity) market.Signal {
			return NewSignal(symbol, entity, system.chart)
		},
		logic.EntityTrade,
	)

	if base == nil {
		return nil
	}

	system.base = base
	system.chart = NewChart(base.Bus(), viper.GetDuration("story.prediction.horizon"))

	return system
}

func (system *System) Tick() error {
	return system.base.Tick()
}

func (system *System) Close() error {
	return system.base.Close()
}
