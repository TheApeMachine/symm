package pumpdump

import (
	"context"
	"fmt"
	"sync"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
	books     broker.BookSource
	pipelines sync.Map
	ID        int
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	level3 := &Level3{
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "pumpdump:level3", level3)
	return level3
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
		// 0. Extract Book Data via Adapter (intercepts measurement, queries book, yields measurement)
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				var bid, ask float64
				level3.books.Book(m.Label, func(b *spotbook.Book) {
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

					m.WriteMetric("best_bid", bid)
					m.WriteMetric("best_ask", ask)
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
				func(m *data.Measurement[float64]) temporal.Observation {
					return temporal.Observation{
						Value: m.GetMetric("midpoint").Raw,
						At:    m.At.UnixNano(),
					}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("midpoint_velocity", out.Rate)
					}
				},
			),
			data.NewAdapter(
				statistic.NewEstimator(),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("spread").Raw
				},
				func(m *data.Measurement[float64], out statistic.MomentReading) {
					if out.VarianceDefined {
						m.WriteMetric("spread_variance", out.Variance)
					}
				},
			),
			data.NewAdapter(
				statistic.NewCUSUM(),
				func(m *data.Measurement[float64]) *statistic.CUSUMObservation {
					return &statistic.CUSUMObservation{
						Sequence:  m.SeqIdx,
						Value:     m.GetMetric("midpoint").Raw,
						Hurdle:    0.0001, // example hurdle
						Threshold: 1.0,
					}
				},
				func(m *data.Measurement[float64], out *statistic.CUSUMReading) {
					m.WriteMetric("cusum_upper", out.UpperSum)
					m.WriteMetric("cusum_lower", out.LowerSum)
				},
			),
			data.NewAdapter(
				probability.NewEntropy(),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("relative_spread").Raw
				},
				func(m *data.Measurement[float64], out *float64) {
					m.WriteMetric("spread_entropy", *out)
				},
			),
		),

		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

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

	measurement.SetSource("pumpdump:level3")

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}
