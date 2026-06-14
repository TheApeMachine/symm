package prediction

import (
	"context"
	"fmt"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
	"github.com/theapemachine/symm/signal"
)

type System struct {
	ctx               context.Context
	base              *signal.System
	chart             *Chart
	featureBus        *internal.Bus
	predictionSignals sync.Map
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
	system.chart = NewChart(base.Bus(), config.DerivedPredictionHorizon())
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

		if rawbus.TypeOf(row) != rawbus.TypeMeasurements {
			continue
		}

		measurement, decodeErr := qpool.ArtifactValue[logic.Measurement](row)

		if decodeErr != nil {
			errnie.Error(fmt.Errorf("prediction: expected logic.Measurement: %w", decodeErr))
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

	raw, err := system.loadPredictionSignal(measurement.Symbol)

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

func (system *System) loadPredictionSignal(symbol string) (market.Signal, error) {
	if raw, ok := system.predictionSignals.Load(symbol); ok {
		if predictionSignal, signalOK := raw.(market.Signal); signalOK {
			return predictionSignal, nil
		}
	}

	predictionSignal, err := system.base.LoadSignal(logic.EntityTrade, symbol)

	if errnie.Error(err) != nil {
		return nil, err
	}

	actual, _ := system.predictionSignals.LoadOrStore(symbol, predictionSignal)

	cached, signalOK := actual.(market.Signal)

	if !signalOK {
		return predictionSignal, nil
	}

	return cached, nil
}
