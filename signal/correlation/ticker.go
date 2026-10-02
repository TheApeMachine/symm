package correlation

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	nmcorrelation "github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the asynchronous price-path correlation instrument. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the
measurement where it computes them, and the workload's register owns the
measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
}

func NewTicker(ctx context.Context, arena *data.ArenaOwner) *Ticker {
	ticker := &Ticker{
		arena: arena,
	}

	ticker.System = runtime.NewSystem(ctx, "correlation:ticker", ticker)
	return ticker
}

func (ticker *Ticker) Source() string {
	return "correlation:ticker"
}

func (ticker *Ticker) Arena() *data.ArenaOwner {
	return ticker.arena
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmcorrelation.NewGate(),
		nmcorrelation.NewPairs(algo.NewHayashiYoshida()),
		nmcorrelation.NewFold(),
		nmcorrelation.NewHistory(),
		nmcorrelation.NewRelative(),
		transport.NewFan(
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("cohort_signed_correlation"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("correlation_velocity", out.Rate)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("relative_return_energy"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("relative_return_energy_velocity", out.Rate)
					}
				},
			),
		),
		nmcorrelation.NewPeerEnergy(),
		data.NewRecurrence(
			"cohort_signed_correlation",
			"relative_return_energy",
			"correlation_velocity",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (ticker *Ticker) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	price := quotedPrice(prior)
	if price <= 0 {
		return nil
	}

	out := ticker.arena.NewMeasurement(ticker.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	out.WriteMetric("last_price", price)

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	if !out.From.IsZero() && out.From.After(out.At) {
		out.From = time.Time{}
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}

func quotedPrice(measurement *data.Measurement[float64]) float64 {
	for _, key := range []string{"last_price", "last", "price"} {
		if metric, ok := measurement.LookupMetric(key); ok && metric.Raw > 0 {
			return metric.Raw
		}
	}

	return 0
}
