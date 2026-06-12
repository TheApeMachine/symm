package prediction

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	ctx        context.Context
	base       *signal.System
	chart      *Chart
	featureBus *internal.Bus
}

func NewSystem(ctx context.Context, pool *qpool.Q[any]) *System {
	system := &System{ctx: ctx}

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
	system.featureBus = internal.NewBus(
		ctx,
		pool,
		nil,
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelMeasurements, "prediction:features"),
		},
	)

	return system
}

func (system *System) Tick() error {
	go func() {
		if err := system.ingestFeatures(); err != nil && !internal.IsShutdown(err) {
			errnie.Error(err)
		}
	}()

	return system.base.Tick()
}

func (system *System) Close() error {
	if system.featureBus != nil {
		if err := system.featureBus.Close(); err != nil {
			return err
		}
	}

	return system.base.Close()
}

func (system *System) ingestFeatures() error {
	for {
		if system.ctx.Err() != nil {
			return system.ctx.Err()
		}

		row, err := system.featureBus.Receive(internal.ChannelMeasurements)

		if internal.IsShutdown(err) {
			return err
		}

		if errnie.Error(err) != nil {
			return err
		}

		if row == nil {
			continue
		}

		if rawbus.TypeFrom(row.Type) != rawbus.TypeMeasurements {
			continue
		}

		measurement, ok := row.Value.(logic.Measurement)

		if !ok {
			if pointerMeasurement, pointerOK := row.Value.(*logic.Measurement); pointerOK && pointerMeasurement != nil {
				measurement = *pointerMeasurement
				ok = true
			}
		}

		if !ok {
			errnie.Error(fmt.Errorf(
				"prediction: expected logic.Measurement, got %T",
				row.Value,
			))
			continue
		}

		if measurement.Source == logic.SourcePrediction {
			continue
		}

		system.applyFeatureMeasurement(measurement)
	}
}

func (system *System) applyFeatureMeasurement(measurement logic.Measurement) {
	sourceIndex := featureSourceIndex(measurement.Source)

	if sourceIndex < 0 {
		return
	}

	raw, err := system.base.LoadSignal(logic.EntityTrade, measurement.Symbol)

	if errnie.Error(err) != nil {
		errnie.Error(err)
		return
	}

	predictionSignal, ok := raw.(*Signal)

	if !ok {
		return
	}

	predictionSignal.recordFeatureMeasurement(measurement)
}
