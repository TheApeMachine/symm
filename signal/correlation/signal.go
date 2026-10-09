package correlation

import (
	"context"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Signal struct {
	*runtime.System
	stages core.Primitive
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
	}

	signal.System = runtime.NewSystem(ctx, "correlation", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	entry := data.Pull(prior.Read("price"))

	if entry == nil || entry.Err != nil || entry.Metric == nil {
		return nil
	}

	price := entry.Metric.Raw

	frame := &nmcorrelation.Frame{
		Symbol:  prior.Label,
		At:      prior.At.UnixNano(),
		Metrics: make(map[string]float64, 36),
	}

	frame.Metrics["last_price"] = price

	for ptr := range signal.stages.Next(transport.NewOne(unsafe.Pointer(frame)).Next(nil)) {
		frame = (*nmcorrelation.Frame)(ptr)
	}

	if err := signal.stages.Error(); err != nil {
		signal.Error(err)
		return nil
	}

	return prior.Next(signal.Name(), frame.Metrics)
}
