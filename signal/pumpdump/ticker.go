package pumpdump

import (
	"context"
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmpumpdump "github.com/theapemachine/symm/nomagique/pumpdump"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the executable-touch market entity. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context, arena *data.ArenaOwner) *Ticker {
	ticker := &Ticker{
		arena: arena,
	}

	ticker.System = runtime.NewSystem(ctx, "pumpdump:ticker", ticker)
	return ticker
}

func (ticker *Ticker) Source() string {
	return "pumpdump:ticker"
}

func (ticker *Ticker) Arena() *data.ArenaOwner {
	return ticker.arena
}

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		// 0. Extract and validate touch geometry
		nmpumpdump.NewTouchGate(),

		// 1. Adaptive baseline and dynamics for relative spread
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("relative_spread").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.WriteMetric("relative_spread_baseline", out.Baseline)
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						if out.Baseline > 0 {
							rs := m.GetMetric("relative_spread").Raw
							spreadRatio := rs / out.Baseline
							m.WriteMetric("spread_ratio", spreadRatio)
							if rs > 0 {
								divergence := math.Log(spreadRatio)
								m.WriteMetric("spread_divergence", divergence)
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(divergence, 'f', -1, 64))
							}
						}

						m.WriteStandardized("spread_zscore", out.ZScore)
						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if div, ok := m.LookupMetric("spread_divergence"); ok {
						return temporal.Observation{Value: div.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("spread_divergence_velocity", out.Rate)
					}
				},
			),
		),

		// 2. Recurrence on spread dynamics
		data.NewRecurrence(
			"spread",
			"relative_spread",
			"spread_divergence",
		),

		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step reads bid/ask from the ingress via StageInput and writes pumpdump metrics
into the owned output measurement. The pipeline's internal closure reads from
the output measurement.
*/
func (ticker *Ticker) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	out := ticker.arena.NewMeasurement(ticker.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	res.Finalize()
	return res
}
