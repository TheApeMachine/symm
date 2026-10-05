package depthflow

import (
	"context"
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	nmdepthflow "github.com/theapemachine/symm/nomagique/depthflow"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the depth-flow measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)
	return signal
}

func (signal *Signal) Arena() *data.ArenaOwner {
	return signal.arena
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (signal *Signal) pipelineFor(symbol string) core.Primitive {
	if existing, ok := signal.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmdepthflow.NewBookFlow(signal.books),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement) float64 {
				return m.GetMetric("book_imbalance").Raw
			},
			func(m *data.Measurement, out adaptive.BaselineReading) {
				m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

				if out.HasPrior {
					m.SetMetric("book_imbalance_baseline", data.NewMetric(
						"book_imbalance_baseline",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("book_imbalance_divergence", data.NewMetric(
						"book_imbalance_divergence",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("book_imbalance_zscore", out.ZScore)
					m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(out.Residual, 'f', -1, 64))

					if out.VarianceDefined {
						m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
					}
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement) float64 {
				return m.GetMetric("imbalance_resolution_gap").Raw
			},
			func(m *data.Measurement, out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("resolution_gap_baseline", data.NewMetric(
						"resolution_gap_baseline",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("resolution_gap_divergence", data.NewMetric(
						"resolution_gap_divergence",
						data.UnitDimensionless,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("resolution_gap_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement) float64 {
				return m.GetMetric("book_turnover_rate").Raw
			},
			func(m *data.Measurement, out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("turnover_baseline", data.NewMetric(
						"turnover_baseline",
						data.UnitRate,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					if out.Baseline > 0 {
						turnover := m.GetMetric("book_turnover_rate").Raw
						m.SetMetric("turnover_ratio", data.NewMetric(
							"turnover_ratio",
							data.UnitRatio,
							data.TimescaleInstantaneous,
							1.0,
							out.ScoreScale/out.Baseline,
						).Write(turnover/out.Baseline))
					}
					m.WriteStandardized("turnover_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement) float64 {
				return m.GetMetric("net_book_change_rate").Raw
			},
			func(m *data.Measurement, out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("net_book_change_rate_baseline", data.NewMetric(
						"net_book_change_rate_baseline",
						data.UnitRate,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("net_book_change_rate_divergence", data.NewMetric(
						"net_book_change_rate_divergence",
						data.UnitRate,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("net_book_change_rate_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement) float64 {
				return m.GetMetric("signed_net_displayed_flow_rate").Raw
			},
			func(m *data.Measurement, out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("signed_net_displayed_flow_rate_baseline", data.NewMetric(
						"signed_net_displayed_flow_rate_baseline",
						data.UnitRate,
						data.TimescaleInstantaneous,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))
					m.SetMetric("signed_net_displayed_flow_rate_divergence", data.NewMetric(
						"signed_net_displayed_flow_rate_divergence",
						data.UnitRate,
						data.TimescaleInstantaneous,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))
					m.WriteStandardized("signed_net_displayed_flow_rate_zscore", out.ZScore)
				}
			},
		),
		data.NewAdapter(
			temporal.NewVelocity(),
			func(m *data.Measurement) temporal.Observation {
				return temporal.Observation{
					Value: m.GetMetric("book_imbalance").Raw,
					At:    m.At.UnixNano(),
				}
			},
			func(m *data.Measurement, out temporal.VelocityReading) {
				if out.Defined {
					m.SetMetric("book_imbalance_velocity", data.NewMetric(
						"book_imbalance_velocity",
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
			func(m *data.Measurement) temporal.Observation {
				return temporal.Observation{
					Value: m.GetMetric("imbalance_resolution_gap").Raw,
					At:    m.At.UnixNano(),
				}
			},
			func(m *data.Measurement, out temporal.VelocityReading) {
				if out.Defined {
					m.SetMetric("resolution_gap_velocity", data.NewMetric(
						"resolution_gap_velocity",
						data.UnitVelocity,
						data.TimescaleInstantaneous,
						0.0,
						0.0,
					).Write(out.Rate))
				}
			},
		),
		data.NewAdapter(
			statistic.NewJoint(4),
			func(m *data.Measurement) statistic.JointInput {
				imb := m.GetMetric("book_imbalance_divergence").Raw
				gap := m.GetMetric("resolution_gap_divergence").Raw
				turn := m.GetMetric("turnover_zscore").Raw
				flow := m.GetMetric("signed_net_displayed_flow_rate_zscore").Raw

				return statistic.JointInput{Values: []float64{imb, gap, turn, flow}}
			},
			func(m *data.Measurement, out statistic.JointReading) {
				if out.SNRDefined && out.SNR < 1/math.Sqrt(2.220446049250313e-16) {
					m.SetMetric("SNR", data.NewMetric(
						"SNR",
						data.UnitSNR,
						data.TimescaleInstantaneous,
						0.0,
						0.0,
					).Write(out.SNR))
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataMahalanobisSNR, strconv.FormatFloat(out.SNR, 'f', -1, 64))
				}
				if len(out.Channels) > 0 {
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Channels[0].Count, 'f', -1, 64))
					n := out.Channels[0].Count
					maturity := 0.0
					if n > 1 {
						maturity = 1.0 - (1.0 / n)
					}
					m.WriteNormalized("Maturity", maturity)
				}
			},
		),
		data.NewRecurrence(
			"book_imbalance_zscore",
			"resolution_gap_zscore",
			"turnover_zscore",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	out := signal.arena.NewMeasurement(signal.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement{prior}

	if channel, hasCh := prior.GetProvenance("channel"); hasCh {
		out.SetProvenance("channel", channel)
	}

	res := data.Read[*data.Measurement](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
