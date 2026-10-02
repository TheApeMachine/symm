package toxicity

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	nmtoxicity "github.com/theapemachine/symm/nomagique/toxicity"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the toxicity measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Level3 {
	level3 := &Level3{
		arena: arena,
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "toxicity:level3", level3)
	return level3
}

func (level3 *Level3) Source() string {
	return "toxicity:level3"
}

func (level3 *Level3) Arena() *data.ArenaOwner {
	return level3.arena
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/

func (level3 *Level3) pipelineFor(symbol string) core.Primitive {
	if existing, ok := level3.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		// 0. Extract raw depth facts and compute touch disposition
		nmtoxicity.NewTouchDisposition(),

		// 1. Baselines and dynamics
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_withdrawal_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("withdrawal_fraction_baseline:bid", out.Baseline)
						m.WriteMetric("withdrawal_fraction_divergence:bid", out.Residual)
						m.WriteStandardized("withdrawal_fraction_zscore:bid", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_withdrawal_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("withdrawal_fraction_baseline:ask", out.Baseline)
						m.WriteMetric("withdrawal_fraction_divergence:ask", out.Residual)
						m.WriteStandardized("withdrawal_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("retreat_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("retreat_fraction_baseline:bid", out.Baseline)
						m.WriteStandardized("retreat_fraction_zscore:bid", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("retreat_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("retreat_fraction_baseline:ask", out.Baseline)
						m.WriteStandardized("retreat_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_replenishment_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("replenishment_fraction_baseline:bid", out.Baseline)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("net_replenishment_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("replenishment_fraction_baseline:ask", out.Baseline)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("net_withdrawal_fraction:bid"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("withdrawal_fraction_velocity:bid", out.Rate)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("net_withdrawal_fraction:ask"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("withdrawal_fraction_velocity:ask", out.Rate)
					}
				},
			),
		),
		// 2. Recurrence on disposition fractions
		data.NewRecurrence(
			"net_withdrawal_fraction:bid",
			"net_withdrawal_fraction:ask",
			"retreat_fraction:bid",
			"retreat_fraction:ask",
		),
		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	out := level3.arena.NewMeasurement(level3.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](level3.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
