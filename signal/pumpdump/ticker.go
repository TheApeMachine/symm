package pumpdump

import (
	"context"
	"fmt"
	"sync"

	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker is the executable-touch market entity. It holds no state and no logic of
its own: its entire behavior is one nomagique pipeline over the measurement itself —
every stage writes its facts into the measurement where it computes them, and the
workload's register owns the measurement's lifetime.
*/
type Ticker struct {
	*runtime.System
	pipelines sync.Map
	ID        int
}

func NewTicker(ctx context.Context) *Ticker {
	ticker := &Ticker{}

	ticker.System = runtime.NewSystem(ctx, "pumpdump:ticker", ticker)
	return ticker
}

func (ticker *Ticker) pipelineFor(symbol string) core.Primitive {
	if existing, ok := ticker.pipelines.Load(symbol); ok {
		return existing.(core.Primitive)
	}

	pipeline := nomagique.NewNumber(
		// 0. Extract source data
		data.NewAdapter(
			transport.NewPass(),
			func(m *data.Measurement[float64]) *data.Measurement[float64] {
				source := m
				if len(m.Peers) > 0 {
					peer := m.FindPeer(func(candidate *data.Measurement[float64]) bool {
						if candidate.Label == "" {
							return false
						}
						bid := candidate.GetMetric("best_bid").Raw
						if bid == 0 {
							bid = candidate.GetMetric("bid").Raw
						}
						ask := candidate.GetMetric("best_ask").Raw
						if ask == 0 {
							ask = candidate.GetMetric("ask").Raw
						}
						return bid > 0 && ask > 0
					})
					if peer != nil {
						source = peer
					}
				}
				m.Pull(source)

				bid := source.GetMetric("best_bid").Raw
				if bid == 0 {
					bid = source.GetMetric("bid").Raw
				}
				ask := source.GetMetric("best_ask").Raw
				if ask == 0 {
					ask = source.GetMetric("ask").Raw
				}

				if bid > 0 && ask > 0 {
					if bid >= ask {
						m.Err = fmt.Errorf("pumpdump: crossed touch (%f >= %f)", bid, ask)
					} else {
						m.WriteMetric("best_bid", bid)
						m.WriteMetric("best_ask", ask)
					}
				}
				m.Label = source.Label
				m.At = source.At
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

		// 2. Compute advanced statistical baseline for relative_spread
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("relative_spread").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.WriteMetric("spread_baseline", out.Baseline)
					m.EnsureMetadata()
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						if out.Baseline > 0 {
							rs := m.GetMetric("relative_spread").Raw
							spreadRatio := rs / out.Baseline
							m.WriteMetric("spread_ratio", spreadRatio)
							if rs > 0 {
								divergence := math.Log(spreadRatio)
								m.WriteMetric("spread_divergence", divergence)
								m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(divergence, 'f', -1, 64))
							}
						}

						m.WriteMetric("spread_zscore", out.ZScore)
						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
			data.NewAdapter(
				temporal.NewVelocity(),
				func(m *data.Measurement[float64]) temporal.Observation {
					if div, ok := m.LookupMetric("spread_divergence"); ok {
						return temporal.Observation{Value: div.Raw, At: m.At.UnixNano()}
					}
					return temporal.Observation{Value: math.NaN(), At: m.At.UnixNano()}
				},
				func(m *data.Measurement[float64], out temporal.VelocityReading) {
					if out.Defined {
						m.WriteMetric("spread_divergence_velocity", out.Rate)
					}
				},
			),
		),
		data.NewFinalizer[float64](),
	)

	actual, _ := ticker.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (ticker *Ticker) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if ticker.Status() != runtime.READY {
		errnie.Warn(ticker.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	measurement.SetSource("pumpdump:ticker")

	res := data.Read[*data.Measurement[float64]](ticker.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))

	if res == nil {
		return nil
	}

	res.Finalize()
	return res
}
