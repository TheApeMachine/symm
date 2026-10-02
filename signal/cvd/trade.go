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
Trade is the CVD executed-flow measuring instrument. It holds no state and no
logic of its own: its entire behavior is composed nomagique pipelines per symbol
over the measurements — every stage writes its facts into the measurement where
it computes them, and the workload's register owns the measurement's lifetime.
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
	trade.System = runtime.NewSystem(ctx, "cvd:trade", trade)
	return trade
}

func (trade *Trade) Source() string {
	return "cvd:trade"
}

func (trade *Trade) Arena() *data.ArenaOwner {
	return trade.arena
}

func (trade *Trade) pipelineFor(symbol string) core.Primitive {
	if existing, ok := trade.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		nmcvd.NewGate(),
		nmcvd.NewQuantity(),
		nmcvd.NewNotional(),
		nmcvd.NewRates(),
		nmcvd.NewResponse(),
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("gross_notional_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("gross_notional_rate_baseline", out.Baseline)
						if out.Baseline > 0 {
							gross := m.GetMetric("gross_notional_rate").Raw
							ratio := gross / out.Baseline
							m.WriteMetric("gross_notional_rate_ratio", ratio)
							if ratio > 0 {
								m.WriteMetric("gross_notional_rate_divergence", math.Log(ratio))
							}
						}
						m.WriteStandardized("gross_notional_rate_zscore", out.ZScore)
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if rate, ok := m.LookupMetric("gross_notional_rate"); ok && rate.Raw > 0 {
						return temporal.Observation{Value: math.Log(rate.Raw), At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: math.NaN(), At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("gross_notional_rate_velocity", out.Rate)
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("midpoint_return_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("midpoint_return_rate_baseline", out.Baseline)
						m.WriteMetric("midpoint_return_rate_divergence", out.Residual)
						m.WriteStandardized("midpoint_return_rate_zscore", out.ZScore)
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
						m.WriteMetric("SNR", out.SNR)
						m.EnsureMetadata()
						m.SetMetadata(data.MetadataMahalanobisSNR, strconv.FormatFloat(out.SNR, 'f', -1, 64))
					}
					if len(out.Channels) > 0 {
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
						m.WriteMetric("flow_response_intercept", out.Fit.Coefficients[0])
						m.WriteMetric("flow_response_coefficient", out.Fit.Coefficients[1])
						if out.PredictionDefined {
							m.WriteMetric("expected_midpoint_return_rate", out.Prediction)
							if y, ok := m.LookupMetric("midpoint_return_rate"); ok {
								m.WriteMetric("flow_response_residual", y.Raw-out.Prediction)
							}
						}
					}
				},
			),
		),
		data.NewRecurrence(
			"gross_notional_rate_zscore",
			"signed_net_fraction_zscore",
			"midpoint_return_rate_zscore",
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := trade.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step reads trade data from the prior measurement and writes CVD metrics
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
