package pumpdump

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the authoritative executable-touch market entity. It holds no state
and no logic of its own: its entire behavior is one nomagique pipeline over the
measurement itself.
*/
type Level3 struct {
	*runtime.System
	books    broker.BookSource
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	level3 := &Level3{
		books: books,
		pipeline: nomagique.NewNumber(
			// 0. Extract Book Data via Adapter (intercepts measurement, queries book, yields measurement)
			data.NewAdapter(
				transport.NewPass(),
				func(m *data.Measurement[float64]) *data.Measurement[float64] {
					var bid, ask float64
					books.Book(m.Label, func(b *spotbook.Book) {
						if b == nil {
							return
						}
						if bestBid := b.BestBid(); bestBid != nil {
							bid = bestBid.Price.Float64()
						}
						if bestAsk := b.BestAsk(); bestAsk != nil {
							ask = bestAsk.Price.Float64()
						}
					})

					if bid > 0 && ask > 0 && bid >= ask {
						m.Err = errnie.Err(
							errnie.Internal,
							fmt.Sprintf("pumpdump: crossed touch (%f >= %f)", bid, ask),
							nil,
						)
					} else if bid > 0 && ask > 0 {
						if m.Metrics == nil {
							m.Metrics = make(map[string]data.Metric[float64])
						}
						m.Metrics["best_bid"] = m.Metrics["best_bid"].Write(bid)
						m.Metrics["best_ask"] = m.Metrics["best_ask"].Write(ask)
					}
					return m
				},
				func(m *data.Measurement[float64], out *data.Measurement[float64]) {},
			),

			// 1. Calculate structural metrics using pure equations
			data.NewEquations(
				data.Equation{
					Output: "spread",
					Op:     arithmetic.NewSubtract(),
					Left:   "best_ask",
					Right:  "best_bid",
				},
				data.Equation{
					Output: "midpoint",
					Op:     arithmetic.NewAdd(),
					Left:   "best_bid",
					Right:  "best_ask",
				},
				data.Equation{
					Output: "relative_spread",
					Op:     arithmetic.NewDivide(),
					Left:   "spread",
					Right:  "midpoint",
				},
			),

			// 2. Compute advanced statistical and temporal features in parallel
			transport.NewFan(
				data.NewAdapter(
					temporal.NewVelocity(),
					func(m *data.Measurement[float64]) *temporal.Observation {
						return &temporal.Observation{
							Value: m.Metrics["midpoint"].Raw,
							At:    m.At.UnixNano(),
						}
					},
					func(m *data.Measurement[float64], out *temporal.VelocityReading) {
						if out.Defined {
							m.Metrics["midpoint_velocity"] = m.Metrics["midpoint_velocity"].Write(out.Rate)
						}
					},
				),
				data.NewAdapter(
					statistic.NewEstimator(),
					func(m *data.Measurement[float64]) *float64 {
						val := m.Metrics["spread"].Raw
						return &val
					},
					func(m *data.Measurement[float64], out *statistic.MomentReading) {
						if out.VarianceDefined {
							m.Metrics["spread_variance"] = m.Metrics["spread_variance"].Write(out.Variance)
						}
					},
				),
				data.NewAdapter(
					statistic.NewCUSUM(),
					func(m *data.Measurement[float64]) *statistic.CUSUMObservation {
						return &statistic.CUSUMObservation{
							Sequence:  m.SeqIdx,
							Value:     m.Metrics["midpoint"].Raw,
							Hurdle:    0.0001, // example hurdle
							Threshold: 1.0,
						}
					},
					func(m *data.Measurement[float64], out *statistic.CUSUMReading) {
						m.Metrics["cusum_upper"] = m.Metrics["cusum_upper"].Write(out.UpperSum)
						m.Metrics["cusum_lower"] = m.Metrics["cusum_lower"].Write(out.LowerSum)
					},
				),
				data.NewAdapter(
					probability.NewEntropy(),
					func(m *data.Measurement[float64]) *float64 {
						val := m.Metrics["relative_spread"].Raw
						return &val
					},
					func(m *data.Measurement[float64], out *float64) {
						m.Metrics["spread_entropy"] = m.Metrics["spread_entropy"].Write(*out)
					},
				),
			),

			// 3. Finalize
			data.NewFinalizer[float64](),
		),
	}

	level3.System = runtime.NewSystem(ctx, "pumpdump:level3", level3)
	return level3
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (level3 *Level3) Step(
	measurement *data.Measurement[float64],
) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil || measurement.Label == "" {
		return measurement
	}

	return data.Read[*data.Measurement[float64]](level3.pipeline.Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("pumpdump:level3", map[string]data.Metric[float64]{
		"best_bid":          data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"best_ask":          data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"midpoint":          data.NewMetric[float64]("midpoint", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"spread":            data.NewMetric[float64]("spread", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"relative_spread":   data.NewMetric[float64]("relative_spread", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"midpoint_velocity": data.NewMetric[float64]("midpoint_velocity", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"spread_variance":   data.NewMetric[float64]("spread_variance", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"cusum_upper":       data.NewMetric[float64]("cusum_upper", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"cusum_lower":       data.NewMetric[float64]("cusum_lower", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"spread_entropy":    data.NewMetric[float64]("spread_entropy", data.UnitNat, data.TimescaleInstantaneous, 0, 0),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
