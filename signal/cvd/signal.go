package cvd

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
	nmcvd "github.com/theapemachine/symm/nomagique/cvd"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is composed nomagique pipelines per symbol
over the measurements — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	pipelines sync.Map
	ID        int
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner) *Signal {
	signal := &Signal{
		arena: arena,
	}
	signal.System = runtime.NewSystem(ctx, "cvd", signal)
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
		nmcvd.NewGate(),
		nmcvd.NewQuantity(),
		nmcvd.NewNotional(),
		nmcvd.NewRates(),
		nmcvd.NewResponse(),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 { return m.GetMetric("gross_notional_rate").Raw },
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("gross_notional_rate_baseline", data.NewMetric[float64](
						"gross_notional_rate_baseline",
						data.UnitNotionalRate,
						data.TimescaleRollingWindow,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))

					if out.Baseline > 0 {
						// Relative dispersion: the baseline's measured spread in units of the baseline.
						relative := out.ScoreScale / out.Baseline
						gross := m.GetMetric("gross_notional_rate").Raw
						ratio := gross / out.Baseline
						m.SetMetric("gross_notional_rate_ratio", data.NewMetric[float64](
							"gross_notional_rate_ratio",
							data.UnitRatio,
							data.TimescaleRollingWindow,
							1.0,
							relative,
						).Write(ratio))

						if ratio > 0 {
							div := math.Log(ratio)
							m.SetMetric("gross_notional_rate_divergence", data.NewMetric[float64](
								"gross_notional_rate_divergence",
								data.UnitPercent,
								data.TimescaleRollingWindow,
								0.0,
								relative,
							).Write(div))
						}
					}

					m.SetMetric("gross_notional_rate_zscore", data.NewMetric[float64](
						"gross_notional_rate_zscore",
						data.UnitStandardDeviation,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(out.ZScore))
				}
			},
		),
		data.NewAdapter(
			temporal.NewVelocity(),
			func(m *data.Measurement[float64]) temporal.Observation {
				return temporal.Observation{
					Value: m.GetMetric("gross_notional_rate").Raw,
					At:    m.At.UnixNano(),
				}
			},
			func(m *data.Measurement[float64], out temporal.VelocityReading) {
				if out.Defined {
					m.SetMetric("gross_notional_rate_velocity", data.NewMetric[float64](
						"gross_notional_rate_velocity",
						data.UnitVelocity,
						data.TimescalePerSecond,
						0.0,
						0.0,
					).Write(out.Rate))
				}
			},
		),
		data.NewAdapter(
			adaptive.NewBaseline(adaptive.NewWindow()),
			func(m *data.Measurement[float64]) float64 { return m.GetMetric("midpoint_return_rate").Raw },
			func(m *data.Measurement[float64], out adaptive.BaselineReading) {
				if out.HasPrior {
					m.SetMetric("midpoint_return_rate_baseline", data.NewMetric[float64](
						"midpoint_return_rate_baseline",
						data.UnitVelocity,
						data.TimescaleRollingWindow,
						out.Baseline,
						out.ScoreScale,
					).Write(out.Baseline))

					m.SetMetric("midpoint_return_rate_divergence", data.NewMetric[float64](
						"midpoint_return_rate_divergence",
						data.UnitVelocity,
						data.TimescaleRollingWindow,
						0.0,
						out.ScoreScale,
					).Write(out.Residual))

					m.SetMetric("midpoint_return_rate_zscore", data.NewMetric[float64](
						"midpoint_return_rate_zscore",
						data.UnitStandardDeviation,
						data.TimescaleRollingWindow,
						0.0,
						1.0,
					).Write(out.ZScore))
				}
			},
		),
		data.NewAdapter(
			statistic.NewJoint(3),
			func(m *data.Measurement[float64]) statistic.JointInput {
				g := m.GetMetric("gross_notional_rate_divergence").Raw
				f := m.GetMetric("signed_net_fraction_divergence").Raw
				r := m.GetMetric("midpoint_return_rate_divergence").Raw
				if g == 0 && f == 0 && r == 0 {
					return statistic.JointInput{Values: nil}
				}
				return statistic.JointInput{Values: []float64{g, f, r}}
			},
			func(m *data.Measurement[float64], out statistic.JointReading) {
				if out.SNRDefined && out.SNR < 1/math.Sqrt(2.220446049250313e-16) {
					m.SetMetric("SNR", data.NewMetric[float64](
						"SNR",
						data.UnitSNR,
						data.TimescaleRollingWindow,
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
		data.NewAdapter(
			statistic.NewRegressionAccumulator(2),
			func(m *data.Measurement[float64]) statistic.RegressionRow {
				if x, ok1 := m.LookupMetric("net_notional_rate"); ok1 {
					if y, ok2 := m.LookupMetric("midpoint_return_rate"); ok2 {
						return statistic.RegressionRow{Predictors: []float64{1.0, x.Raw}, Target: y.Raw}
					}
				}
				return statistic.RegressionRow{Predictors: nil, Target: math.NaN()}
			},
			func(m *data.Measurement[float64], out statistic.RegressionReading) {
				if out.Fit.Observations > 2 && len(out.Fit.Coefficients) >= 2 {
					m.SetMetric("flow_response_intercept", data.NewMetric[float64](
						"flow_response_intercept",
						data.UnitVelocity,
						data.TimescaleRollingWindow,
						0.0,
						0.0,
					).Write(out.Fit.Coefficients[0]))

					m.SetMetric("flow_response_coefficient", data.NewMetric[float64](
						"flow_response_coefficient",
						data.UnitRatio,
						data.TimescaleRollingWindow,
						0.0,
						0.0,
					).Write(out.Fit.Coefficients[1]))

					if out.PredictionDefined {
						m.SetMetric("expected_midpoint_return_rate", data.NewMetric[float64](
							"expected_midpoint_return_rate",
							data.UnitVelocity,
							data.TimescaleTick,
							0.0,
							0.0,
						).Write(out.Prediction))

						if y, ok := m.LookupMetric("midpoint_return_rate"); ok {
							residual := y.Raw - out.Prediction
							m.SetMetric("flow_response_residual", data.NewMetric[float64](
								"flow_response_residual",
								data.UnitVelocity,
								data.TimescaleTick,
								0.0,
								0.0,
							).Write(residual))
						}
					}
				}
			},
		),
		data.NewRecurrence(
			"gross_notional_rate_zscore",
			"signed_net_fraction_zscore",
			"midpoint_return_rate_zscore",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := signal.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step reads trade data from the prior measurement and writes CVD metrics
into a fresh measurement allocated from its own arena.
*/
func (signal *Signal) Step(prior *data.Measurement[float64]) *data.Measurement[float64] {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
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

	out := signal.arena.NewMeasurement(signal.Name())
	out.Epoch = prior.Epoch
	out.Tick = prior.Tick
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

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
