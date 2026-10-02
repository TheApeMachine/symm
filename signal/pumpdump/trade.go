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
Trade owns the volume-clock activity pipeline. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
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

	trade.System = runtime.NewSystem(ctx, "pumpdump:trade", trade)
	return trade
}

func (trade *Trade) Source() string {
	return "pumpdump:trade"
}

func (trade *Trade) Arena() *data.ArenaOwner {
	return trade.arena
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		// 0. Volume clock
		nmpumpdump.NewVolumeClock(),

		// 1. Adaptive baselines and temporal dynamics
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("notional_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("notional_rate_baseline", out.Baseline)
						if out.Baseline > 0 {
							ratio := m.GetMetric("notional_rate").Raw / out.Baseline
							m.WriteMetric("notional_rate_ratio", ratio)
							if ratio > 0 {
								div := math.Log(ratio)
								m.WriteMetric("notional_rate_divergence", div)
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(div, 'f', -1, 64))
							}
						}
						m.WriteStandardized("notional_rate_zscore", out.ZScore)
						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("notional_rate"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("notional_rate_velocity", out.Rate)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("midpoint_return_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("midpoint_return_baseline", out.Baseline)
						m.WriteMetric("midpoint_return_divergence", out.Residual)
						m.WriteStandardized("midpoint_return_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("midpoint_return_rate"); ok {
						return temporal.Observation{Value: rate.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: 0, At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("midpoint_return_velocity", out.Rate)
					}
				},
			),
		),

		// 2. Recurrence on volume-clock dynamics
		data.NewRecurrence(
			"notional_rate",
			"volume_rate",
			"midpoint_return_rate",
		),

		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (trade *Trade) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if trade.Status() != runtime.READY {
		errnie.Warn(trade.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
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

	res := data.Read[*data.Measurement[float64]](trade.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	res.Finalize()
	return res
}
