package correlation

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/data"
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

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := data.Attribute[float64](
		value.Layers[0], "data", 0, "price",
	)

	if price <= 0 {
		return value
	}

	frame := &nmcorrelation.Frame{
		Symbol:  value.Scope,
		At:      int64(value.Timestamp),
		Metrics: map[string]float64{"last_price": price},
	}

	for ptr := range signal.stages.Next(transport.NewOne(
		unsafe.Pointer(frame),
	).Next(nil)) {
		frame = (*nmcorrelation.Frame)(ptr)
	}

	return data.AppendVector(
		value, []float64{
			frame.Metrics[signal.keys[0]],
			frame.Metrics[signal.keys[1]],
			frame.Metrics[signal.keys[2]],
		}, signal.keys...,
	)
}
