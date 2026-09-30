package depthflow

import (
	"context"
	"math"
	"strconv"
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
	pipeline core.Primitive
	ID       int
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {
	prevTimes := make(map[string]time.Time)

	level3 := &Level3{
		pipeline: nomagique.NewNumber(
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
							_, hasObsBid := p.Metrics["observed_notional:bid"]
							_, hasObsAsk := p.Metrics["observed_notional:ask"]
							if hasObsBid || hasObsAsk {
								return true
							}
							b := p.Metrics["best_bid"].Raw
							if b == 0 {
								b = p.Metrics["bid"].Raw
							}
							a := p.Metrics["best_ask"].Raw
							if a == 0 {
								a = p.Metrics["ask"].Raw
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

					obsBid := input.Metrics["observed_notional:bid"].Raw
					obsAsk := input.Metrics["observed_notional:ask"].Raw
					mutBid := input.Metrics["mutation_count:bid"].Raw
					mutAsk := input.Metrics["mutation_count:ask"].Raw

					if obsBid == 0 && obsAsk == 0 {
						bidPrice := input.Metrics["best_bid"].Raw
						if bidPrice == 0 {
							bidPrice = input.Metrics["bid"].Raw
						}
						askPrice := input.Metrics["best_ask"].Raw
						if askPrice == 0 {
							askPrice = input.Metrics["ask"].Raw
						}
						bidQty := input.Metrics["touch_quantity:bid"].Raw
						if bidQty == 0 {
							bidQty = input.Metrics["bid_qty"].Raw
						}
						askQty := input.Metrics["touch_quantity:ask"].Raw
						if askQty == 0 {
							askQty = input.Metrics["ask_qty"].Raw
						}
						if bidPrice > 0 && bidQty > 0 {
							obsBid = bidPrice * bidQty
							mutBid = 1
						}
						if askPrice > 0 && askQty > 0 {
							obsAsk = askPrice * askQty
							mutAsk = 1
						}
					}

					if m.Metrics == nil {
						m.Metrics = make(map[string]data.Metric[float64])
					}

					if obsBid > 0 || obsAsk > 0 {
						m.Metrics["observed_notional:bid"] = m.Metrics["observed_notional:bid"].Write(obsBid)
						m.Metrics["observed_notional:ask"] = m.Metrics["observed_notional:ask"].Write(obsAsk)
						m.Metrics["mutation_count:bid"] = m.Metrics["mutation_count:bid"].Write(mutBid)
						m.Metrics["mutation_count:ask"] = m.Metrics["mutation_count:ask"].Write(mutAsk)

						observed := obsBid + obsAsk
						if prevTime, ok := prevTimes[m.Label]; ok {
							dt := m.At.Sub(prevTime).Seconds()
							if dt > 0 {
								effectiveDt := math.Max(dt, 1.0)
								rate := observed / effectiveDt
								m.Metrics["observed_notional_rate"] = m.Metrics["observed_notional_rate"].Write(rate)
							}
						}
						prevTimes[m.Label] = m.At
					}

					if m.Metadata == nil {
						m.Metadata = make(map[string]string)
					}

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
					func(m *data.Measurement[float64]) *float64 {
						val := m.Metrics["observed_notional_imbalance"].Raw
						return &val
					},
					func(m *data.Measurement[float64], out *adaptive.BaselineReading) {
						m.Metadata[data.MetadataSupport] = strconv.FormatFloat(out.Count, 'f', -1, 64)

						if out.HasPrior {
							m.Metrics["observed_notional_imbalance_baseline"] = m.Metrics["observed_notional_imbalance_baseline"].Write(out.Baseline)
							m.Metrics["observed_notional_imbalance_divergence"] = m.Metrics["observed_notional_imbalance_divergence"].Write(out.Residual)
							m.Metrics["observed_notional_imbalance_zscore"] = m.Metrics["observed_notional_imbalance_zscore"].Write(out.ZScore)
							m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(out.Residual, 'f', -1, 64)

							if out.VarianceDefined {
								m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(out.Variance, 'f', -1, 64)
							}
						}
					},
				),
				data.NewAdapter(
					adaptive.NewBaseline(adaptive.NewWindow()),
					func(m *data.Measurement[float64]) *float64 {
						val := m.Metrics["observed_notional_rate"].Raw
						return &val
					},
					func(m *data.Measurement[float64], out *adaptive.BaselineReading) {
						if out.HasPrior {
							m.Metrics["observed_notional_rate_baseline"] = m.Metrics["observed_notional_rate_baseline"].Write(out.Baseline)
							m.Metrics["observed_notional_rate_divergence"] = m.Metrics["observed_notional_rate_divergence"].Write(out.Residual)
							m.Metrics["observed_notional_rate_zscore"] = m.Metrics["observed_notional_rate_zscore"].Write(out.ZScore)
						}
					},
				),
			),
			// 3. Finalize
			data.NewFinalizer[float64](),
		),
	}

	level3.System = runtime.NewSystem(ctx, "depthflow:level3", level3)
	return level3
}

/*
Step supplies the arriving measurement to the pipeline and returns it: the
measurement is the pipeline's state, enriched in place.
*/
func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return m
	}

	if m == nil || m.Err != nil {
		return m
	}

	return data.Read[*data.Measurement[float64]](level3.pipeline.Next(
		transport.NewOne(unsafe.Pointer(&m)).Next(nil),
	))
}

/*
Register returns the measurement declaring this entity's full metric schema.
Values are empty; the workload uses this at startup to allocate the metric
schema before feeding streaming records.
*/
func (level3 *Level3) Register() *data.Measurement[float64] {
	m := data.NewMeasurement("depthflow:level3", map[string]data.Metric[float64]{
		"observed_notional:bid":                  data.NewMetric[float64]("observed_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional:ask":                  data.NewMetric[float64]("observed_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional":                      data.NewMetric[float64]("observed_notional", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_diff":                 data.NewMetric[float64]("observed_notional_diff", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"add_notional:bid":                       data.NewMetric[float64]("add_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"add_notional:ask":                       data.NewMetric[float64]("add_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"modify_remaining_notional:bid":          data.NewMetric[float64]("modify_remaining_notional:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"modify_remaining_notional:ask":          data.NewMetric[float64]("modify_remaining_notional:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"delete_count:bid":                       data.NewMetric[float64]("delete_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"delete_count:ask":                       data.NewMetric[float64]("delete_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"mutation_count:bid":                     data.NewMetric[float64]("mutation_count:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"mutation_count:ask":                     data.NewMetric[float64]("mutation_count:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"mutation_count":                         data.NewMetric[float64]("mutation_count", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"mutation_count_diff":                    data.NewMetric[float64]("mutation_count_diff", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"mutation_activity_imbalance":            data.NewMetric[float64]("mutation_activity_imbalance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_imbalance":            data.NewMetric[float64]("observed_notional_imbalance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate":                 data.NewMetric[float64]("observed_notional_rate", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_imbalance_baseline":   data.NewMetric[float64]("observed_notional_imbalance_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_imbalance_divergence": data.NewMetric[float64]("observed_notional_imbalance_divergence", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_imbalance_zscore":     data.NewMetric[float64]("observed_notional_imbalance_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
		"observed_notional_rate_baseline":        data.NewMetric[float64]("observed_notional_rate_baseline", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_rate_divergence":      data.NewMetric[float64]("observed_notional_rate_divergence", data.UnitRate, data.TimescaleInstantaneous, 0, 0),
		"observed_notional_rate_zscore":          data.NewMetric[float64]("observed_notional_rate_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
