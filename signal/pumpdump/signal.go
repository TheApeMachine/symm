package pumpdump

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
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Signal is the unified pumpdump / volume-clocked activity measuring instrument.
It captures book touch geometry and spread dynamics alongside trade-arrival
volume-clock dynamics, outputting exactly one measurement per trade tick.
*/
type Signal struct {
	*runtime.System
	arena     *data.ArenaOwner
	books     broker.BookSource
	pipelines sync.Map
	ID        int
}

func NewSignal(ctx context.Context, arena *data.ArenaOwner, books broker.BookSource) *Signal {
	signal := &Signal{
		arena: arena,
		books: books,
	}

	signal.System = runtime.NewSystem(ctx, "pumpdump", signal)
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
		// 0. Extract and validate book touch geometry
		NewBookTouch(signal.books),

		// 1. Volume clock for trade dynamics
		NewVolumeClock(),

		// 2. Adaptive baselines and temporal dynamics
		transport.NewFan(
			// Spread baseline & divergence
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("relative_spread").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						m.SetMetric("relative_spread_baseline", data.NewMetric[float64](
							"relative_spread_baseline",
							data.UnitRelativeSpread,
							data.TimescaleRollingWindow,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))

						if out.Baseline > 0 {
							// Relative dispersion: the baseline's measured spread in units of the baseline.
							relative := out.ScoreScale / out.Baseline
							rs := m.GetMetric("relative_spread").Raw
							spreadRatio := rs / out.Baseline
							m.SetMetric("spread_ratio", data.NewMetric[float64](
								"spread_ratio",
								data.UnitRatio,
								data.TimescaleRollingWindow,
								1.0,
								relative,
							).Write(spreadRatio))

							if rs > 0 {
								divergence := math.Log(spreadRatio)
								m.SetMetric("spread_divergence", data.NewMetric[float64](
									"spread_divergence",
									data.UnitRelativeSpread,
									data.TimescaleRollingWindow,
									0.0,
									relative,
								).Write(divergence))
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(divergence, 'f', -1, 64))
							}
						}

						m.SetMetric("spread_zscore", data.NewMetric[float64](
							"spread_zscore",
							data.UnitStandardDeviation,
							data.TimescaleRollingWindow,
							0.0,
							1.0,
						).Write(out.ZScore))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			// Spread velocity
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
						m.SetMetric("spread_divergence_velocity", data.NewMetric[float64](
							"spread_divergence_velocity",
							data.UnitVelocity,
							data.TimescalePerSecond,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
			// Notional rate baseline & divergence
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("notional_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.SetMetric("notional_rate_baseline", data.NewMetric[float64](
							"notional_rate_baseline",
							data.UnitNotionalRate,
							data.TimescaleRollingWindow,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))

						if out.Baseline > 0 {
							// Relative dispersion: the baseline's measured spread in units of the baseline.
							relative := out.ScoreScale / out.Baseline
							ratio := m.GetMetric("notional_rate").Raw / out.Baseline
							m.SetMetric("notional_rate_ratio", data.NewMetric[float64](
								"notional_rate_ratio",
								data.UnitRatio,
								data.TimescaleRollingWindow,
								1.0,
								relative,
							).Write(ratio))

							if ratio > 0 {
								div := math.Log(ratio)
								m.SetMetric("notional_rate_divergence", data.NewMetric[float64](
									"notional_rate_divergence",
									data.UnitNotionalRate,
									data.TimescaleRollingWindow,
									0.0,
									relative,
								).Write(div))
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(div, 'f', -1, 64))
							}
						}

						m.SetMetric("notional_rate_zscore", data.NewMetric[float64](
							"notional_rate_zscore",
							data.UnitStandardDeviation,
							data.TimescaleRollingWindow,
							0.0,
							1.0,
						).Write(out.ZScore))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}

					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))
				},
			),
			// Notional rate velocity
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
						m.SetMetric("notional_rate_velocity", data.NewMetric[float64](
							"notional_rate_velocity",
							data.UnitVelocity,
							data.TimescalePerSecond,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
			// Midpoint return baseline
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 { return m.GetMetric("midpoint_return_rate").Raw },
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.SetMetric("midpoint_return_baseline", data.NewMetric[float64](
							"midpoint_return_baseline",
							data.UnitVelocity,
							data.TimescaleRollingWindow,
							out.Baseline,
							out.ScoreScale,
						).Write(out.Baseline))

						m.SetMetric("midpoint_return_divergence", data.NewMetric[float64](
							"midpoint_return_divergence",
							data.UnitVelocity,
							data.TimescaleRollingWindow,
							0.0,
							out.ScoreScale,
						).Write(out.Residual))

						m.SetMetric("midpoint_return_zscore", data.NewMetric[float64](
							"midpoint_return_zscore",
							data.UnitStandardDeviation,
							data.TimescaleRollingWindow,
							0.0,
							1.0,
						).Write(out.ZScore))
					}
				},
			),
			// Midpoint return velocity
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
						m.SetMetric("midpoint_return_velocity", data.NewMetric[float64](
							"midpoint_return_velocity",
							data.UnitAcceleration,
							data.TimescalePerSecond,
							0.0,
							0.0,
						).Write(out.Rate))
					}
				},
			),
		),

		// 3. Recurrence
		data.NewRecurrence(
			"spread",
			"relative_spread",
			"spread_divergence",
			"notional_rate",
			"volume_rate",
			"midpoint_return_rate",
		),

		// 4. Finalize
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

	// Copy trade metrics from prior
	if price, ok := prior.LookupMetric("price"); ok {
		out.SetMetric("price", price)
	}
	if qty, ok := prior.LookupMetric("qty"); ok {
		out.SetMetric("qty", qty)
	}
	if bestBid, ok := prior.LookupMetric("best_bid"); ok {
		out.SetMetric("best_bid", bestBid)
	}
	if bestAsk, ok := prior.LookupMetric("best_ask"); ok {
		out.SetMetric("best_ask", bestAsk)
	}

	res := data.Read[*data.Measurement[float64]](signal.pipelineFor(out.Label).Next(
		transport.NewOne(unsafe.Pointer(&out)).Next(nil),
	))

	if res == nil {
		return out
	}

	return res
}
