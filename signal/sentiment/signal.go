package sentiment

import (
	"context"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/crosssection"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmsentiment "github.com/theapemachine/symm/nomagique/sentiment"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	prices    *store.Latest[string, float64]
	changes   *store.Latest[string, data.CrossMember]
	ID        int
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena:   arena,
		prices:  store.NewLatest[string, float64](),
		changes: store.NewLatest[string, data.CrossMember](),
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
}

func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

func (signal *Signal) pipelineFor(symbol string) core.Primitive {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		data.NewMetricGate("last"),
		crosssection.NewUpdateMember("last", signal.prices, signal.changes),
		crosssection.NewStampPeers(signal.changes),
		nmsentiment.NewCrossSentiment(),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 {
				return m.GetMetric("median_return").Raw
			},
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("median_return_baseline", data.NewMetric[float64](
						"median_return_baseline",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("median_return_divergence", data.NewMetric[float64](
						"median_return_divergence",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("median_return_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 {
				return m.GetMetric("breadth").Raw
			},
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("breadth_baseline", data.NewMetric[float64](
						"breadth_baseline",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("breadth_divergence", data.NewMetric[float64](
						"breadth_divergence",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("breadth_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 {
				return m.GetMetric("median_absolute_return").Raw
			},
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("median_absolute_return_baseline", data.NewMetric[float64](
						"median_absolute_return_baseline",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					if out.Baseline > 0 {
						m.SetMetric("median_absolute_return_ratio", data.NewMetric[float64](
							"median_absolute_return_ratio",
							data.UnitRatio,
							data.TimescaleInstantaneous,
							1.0,
							out.ScoreScale/out.Baseline,
						).Write(m.GetMetric("median_absolute_return").Raw/out.Baseline))
					}
					m.WriteStandardized("median_absolute_return_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 {
				return m.GetMetric("return_mad").Raw
			},
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("return_dispersion_baseline", data.NewMetric[float64](
						"return_dispersion_baseline",
						data.UnitLogReturn,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					if out.Baseline > 0 {
						m.SetMetric("return_dispersion_ratio", data.NewMetric[float64](
							"return_dispersion_ratio",
							data.UnitRatio,
							data.TimescaleInstantaneous,
							1.0,
							out.ScoreScale/out.Baseline,
						).Write(m.GetMetric("return_mad").Raw/out.Baseline))
					}
					m.WriteStandardized("return_dispersion_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			temporal.NewVelocity(),
			func(m *data.Measurement[float64]) temporal.Observation {
				return temporal.Observation{
					Value: m.GetMetric("median_return").Raw,
					At:    m.At.UnixNano(),
				}
			},
			func(m *data.Measurement[float64], out temporal.VelocityReading) {
				if out.Defined {
					m.SetMetric("median_return_velocity", data.NewMetric[float64](
						"median_return_velocity",
						data.UnitVelocity,
						data.TimescaleInstantaneous,
						0.0,
						0.0,
					).Write(out.Rate))
				}
			},
		),
		data.NewAdapter(
			temporal.NewVelocity(),
			func(m *data.Measurement[float64]) temporal.Observation {
				return temporal.Observation{
					Value: m.GetMetric("breadth").Raw,
					At:    m.At.UnixNano(),
				}
			},
			func(m *data.Measurement[float64], out temporal.VelocityReading) {
				if out.Defined {
					m.SetMetric("breadth_velocity", data.NewMetric[float64](
						"breadth_velocity",
						data.UnitVelocity,
						data.TimescaleInstantaneous,
						0.0,
						0.0,
					).Write(out.Rate))
				}
			},
		),
		data.NewRecurrence(
			"median_return",
			"breadth",
			"return_mad",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (signal *Signal) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	priceMetric, hasPrice := quotedPriceMetric(prior)
	if !hasPrice || priceMetric.Raw <= 0 {
		return nil
	}

	out := signal.arena.NewMeasurement(signal.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement[float64]{prior}

	out.SetMetric("last", priceMetric)

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}

func quotedPriceMetric(measurement *data.Measurement[float64]) (data.Metric[float64], bool) {
	for _, key := range []string{"last", "last_price", "price"} {
		if metric, ok := measurement.LookupMetric(key); ok && metric.Raw > 0 {
			return metric, true
		}
	}

	return data.Metric[float64]{}, false
}
