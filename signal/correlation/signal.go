package correlation

import (
	"context"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Signal struct {
	*runtime.System
	stages core.Primitive
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		stages: transport.NewStages(
			nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
			nmcorrelation.NewFold(),
			nmcorrelation.NewRelative(),
			nmcorrelation.NewHistory(),
			nmcorrelation.NewCorrelationVelocity(),
			nmcorrelation.NewEnergyVelocity(),
			nmcorrelation.NewRecurrence(),
		),
		keys: []string{
			"cohort_covariance_score",
			"historical_path_distance",
			"historical_path_percentile",
		},
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	trades, err := data.AttributeOk[[]kraken.TradeData](value.Layers[0], "data")

	if err != nil {
		errnie.Error(err)
		return value
	}

	for _, trade := range trades {
		signal.step(value, trade)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := trade.Price.Float64()

	if price <= 0 {
		return value
	}

	frame := &nmcorrelation.Frame{
		Symbol:  trade.Symbol,
		At:      int64(value.Timestamp),
		Metrics: map[string]float64{"last_price": price},
	}

	for ptr := range signal.stages.Next(transport.NewOne(
		unsafe.Pointer(frame),
	).Next(nil)) {
		frame = (*nmcorrelation.Frame)(ptr)
	}

	return data.AppendSignalVector(value, signal.Name(), trade.Symbol, []float64{
		frame.Metrics[signal.keys[0]],
		frame.Metrics[signal.keys[1]],
		frame.Metrics[signal.keys[2]],
	}, signal.keys...,
	)
}
