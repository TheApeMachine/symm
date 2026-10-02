package toxicity

import (
	"context"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

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
Trade matches incoming trades against the symbol's book touch. It holds no
state and no logic of its own: its entire behavior is one nomagique pipeline
over the measurement itself — every stage writes its facts into the measurement
where it computes them, and the workload's register owns the measurement's lifetime.
*/
type Trade struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewTrade(ctx context.Context, arena *data.ArenaOwner) *Trade {
	trade := &Trade{
		arena: arena,
	}

	trade.System = runtime.NewSystem(ctx, "toxicity:trade", trade)
	return trade
}

func (trade *Trade) Source() string {
	return "toxicity:trade"
}

func (trade *Trade) Arena() *data.ArenaOwner {
	return trade.arena
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmtoxicity.NewTradeMatching(),
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("touch_fill_fraction:bid").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("fill_fraction_baseline:bid", out.Baseline)
						m.WriteMetric("fill_fraction_divergence:bid", out.Residual)
						m.WriteStandardized("fill_fraction_zscore:bid", out.ZScore)
					}
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))
					if out.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("touch_fill_fraction:ask").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("fill_fraction_baseline:ask", out.Baseline)
						m.WriteMetric("fill_fraction_divergence:ask", out.Residual)
						m.WriteStandardized("fill_fraction_zscore:ask", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("touch_fill_fraction:bid"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("fill_fraction_velocity:bid", out.Rate)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if v, ok := m.LookupMetric("touch_fill_fraction:ask"); ok {
						return temporal.Observation{Value: v.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("fill_fraction_velocity:ask", out.Rate)
					}
				},
			),
		),
		data.NewRecurrence(
			"touch_fill_fraction:bid",
			"touch_fill_fraction:ask",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step reads trade data from the prior measurement and writes toxicity metrics
into a fresh measurement allocated from its own arena.
*/
func (trade *Trade) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	_, hasPrice := prior.LookupMetric("price")
	_, hasQty := prior.LookupMetric("qty")

	if !hasPrice || !hasQty {
		return nil
	}

	out := trade.arena.NewMeasurement(trade.Source())
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	if side, hasSide := prior.GetProvenance("side"); hasSide {
		out.SetProvenance("side", side)
	}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	if out.From.IsZero() {
		out.From = out.At
	}

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
