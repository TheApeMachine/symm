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
	last   map[string]*data.Measurement
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
		last: make(map[string]*data.Measurement),
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
		if prev, ok := signal.last[prior.Label]; ok && prev != nil {
			return prev
		}

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

	var out *data.Measurement
	if prev, ok := signal.last[prior.Label]; ok {
		out = prev.Next(prior.At, frame.Metrics)
	}

	if out == nil {
		out = data.NewFromPrior(prior, signal.Name(), frame.Metrics)
	}

	if frame.Reference != "" {
		out.SetMeta(data.MetadataReferencePeer, frame.Reference)
	}

	signal.last[prior.Label] = out
	return out
}
