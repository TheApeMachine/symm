package depthflow

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the depth-flow measuring instrument. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Level3 struct {
	*runtime.System
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	level3 := &Level3{
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "depthflow:level3", level3)
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

	var prevTime time.Time

	pipeline := nomagique.NewNumber(
		// 0. Extract raw depth facts from peers or self
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				input := m

				if len(m.Peers) > 0 {
					peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
						if p.Label == "" {
							return false
						}
						_, hasObsBid := p.LookupMetric("observed_notional:bid")
						_, hasObsAsk := p.LookupMetric("observed_notional:ask")
						if hasObsBid || hasObsAsk {
							return true
						}
						b := p.GetMetric("best_bid").Raw
						if b == 0 {
							b = p.GetMetric("bid").Raw
						}
						a := p.GetMetric("best_ask").Raw
						if a == 0 {
							a = p.GetMetric("ask").Raw
						}
						return b > 0 && a > 0
					})

					if peer != nil {
						input = peer
					}
				}

				if input.Label != "" {
					m.Label = input.Label
				}
				if !input.At.IsZero() {
					m.At = input.At
				}

				obsBid := input.GetMetric("observed_notional:bid").Raw
				obsAsk := input.GetMetric("observed_notional:ask").Raw
				mutBid := input.GetMetric("mutation_count:bid").Raw
				mutAsk := input.GetMetric("mutation_count:ask").Raw

				if m.Metrics == nil {
					m.Metrics = make(map[string]data.Metric[float64])
				}

				if obsBid > 0 || obsAsk > 0 {
					m.WriteMetric("observed_notional:bid", obsBid)
					m.WriteMetric("observed_notional:ask", obsAsk)
					m.WriteMetric("mutation_count:bid", mutBid)
					m.WriteMetric("mutation_count:ask", mutAsk)

					observed := obsBid + obsAsk
					if !prevTime.IsZero() {
						dt := m.At.Sub(prevTime).Seconds()
						if dt > 0 {
							effectiveDt := math.Max(dt, 1.0)
							rate := observed / effectiveDt
							m.WriteMetric("observed_notional_rate", rate)
						}
					}
					prevTime = m.At
				}

				m.EnsureMetadata()

				return m
			},
			func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
		),
		// 1. Calculate structural metrics using pure equations
		data.NewEquations(
			data.Equation{
				Output: "observed_notional",
				Op:     arithmetic.NewAdd(),
				Left:   "observed_notional:bid",
				Right:  "observed_notional:ask",
			},
			data.Equation{
				Output: "observed_notional_diff",
				Op:     arithmetic.NewSubtract(),
				Left:   "observed_notional:bid",
				Right:  "observed_notional:ask",
			},
			data.Equation{
				Output: "mutation_count",
				Op:     arithmetic.NewAdd(),
				Left:   "mutation_count:bid",
				Right:  "mutation_count:ask",
			},
			data.Equation{
				Output: "mutation_count_diff",
				Op:     arithmetic.NewSubtract(),
				Left:   "mutation_count:bid",
				Right:  "mutation_count:ask",
			},
			data.Equation{
				Output: "observed_notional_imbalance",
				Op:     arithmetic.NewDivide(),
				Left:   "observed_notional_diff",
				Right:  "observed_notional",
			},
			data.Equation{
				Output: "mutation_activity_imbalance",
				Op:     arithmetic.NewDivide(),
				Left:   "mutation_count_diff",
				Right:  "mutation_count",
			},
		),
		// 2. Baselines
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("observed_notional_imbalance").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						m.WriteMetric("observed_notional_imbalance_baseline", out.Baseline)
						m.WriteMetric("observed_notional_imbalance_divergence", out.Residual)
						m.WriteMetric("observed_notional_imbalance_zscore", out.ZScore)
						m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(out.Residual, 'f', -1, 64))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("observed_notional_rate").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					if out.HasPrior {
						m.WriteMetric("observed_notional_rate_baseline", out.Baseline)
						m.WriteMetric("observed_notional_rate_divergence", out.Residual)
						m.WriteMetric("observed_notional_rate_zscore", out.ZScore)
					}
				},
			),
		),
		// 3. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	measurement.Source = "depthflow:level3"

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}
