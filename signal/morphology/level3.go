package morphology

import (
	"context"
	"math"
	"strconv"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Level3 is the book-morphology measuring instrument. It holds no state and no logic of
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
	type morphologyState struct {
		hasPrev      bool
		prevDistance float64
	}
	states := make(map[string]*morphologyState)

	level3 := &Level3{
		pipeline: nomagique.NewNumber(
			// 0. Extract raw morphology facts from peers or self
			data.NewAdapter(
				transport.NewPass(),
				func(m *data.Measurement[float64]) *data.Measurement[float64] {
					input := m

					if len(m.Peers) > 0 {
						peer := m.FindPeer(func(p *data.Measurement[float64]) bool {
							if p.Label == "" {
								return false
							}
							_, hasDist := p.Metrics["book_shape_distance"]
							if hasDist {
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

					distance := input.Metrics["book_shape_distance"].Raw
					ks := input.Metrics["book_shape_ks"].Raw
					concBid := input.Metrics["concentration:bid"].Raw
					concAsk := input.Metrics["concentration:ask"].Raw
					entBid := input.Metrics["entropy:bid"].Raw
					entAsk := input.Metrics["entropy:ask"].Raw

					if distance == 0 {
						b := input.Metrics["best_bid"].Raw
						if b == 0 {
							b = input.Metrics["bid"].Raw
						}
						a := input.Metrics["best_ask"].Raw
						if a == 0 {
							a = input.Metrics["ask"].Raw
						}
						mid := (b + a) / 2.0
						if mid > 0 {
							distance = (a - b) / mid
						}
					}

					if m.Metrics == nil {
						m.Metrics = make(map[string]data.Metric[float64])
					}

					if distance > 0 {
						m.Metrics["book_shape_distance"] = m.Metrics["book_shape_distance"].Write(distance)
						m.Metrics["book_shape_ks"] = m.Metrics["book_shape_ks"].Write(ks)
						m.Metrics["concentration:bid"] = m.Metrics["concentration:bid"].Write(concBid)
						m.Metrics["concentration:ask"] = m.Metrics["concentration:ask"].Write(concAsk)
						m.Metrics["entropy:bid"] = m.Metrics["entropy:bid"].Write(entBid)
						m.Metrics["entropy:ask"] = m.Metrics["entropy:ask"].Write(entAsk)

						state := states[m.Label]
						if state == nil {
							state = &morphologyState{}
							states[m.Label] = state
						}

						if state.hasPrev {
							change := math.Abs(distance - state.prevDistance)
							m.Metrics["morphology_change"] = m.Metrics["morphology_change"].Write(change)
						}

						state.prevDistance = distance
						state.hasPrev = true
					}

					if m.Metadata == nil {
						m.Metadata = make(map[string]string)
					}

					return m
				},
				func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
			),
			// 1. Baselines
			transport.NewFan(
				data.NewAdapter(
					adaptive.NewBaseline(adaptive.NewWindow()),
					func(m *data.Measurement[float64]) *float64 {
						changeMetric, ok := m.Metrics["morphology_change"]
						if !ok {
							return nil
						}
						val := changeMetric.Raw
						return &val
					},
					func(m *data.Measurement[float64], out *adaptive.BaselineReading) {
						if out == nil {
							return
						}
						m.Metadata[data.MetadataSupport] = strconv.FormatFloat(out.Count, 'f', -1, 64)

						if out.HasPrior {
							m.Metrics["morphology_change_baseline"] = m.Metrics["morphology_change_baseline"].Write(out.Baseline)
							m.Metrics["morphology_change_zscore"] = m.Metrics["morphology_change_zscore"].Write(out.ZScore)
							m.Metadata[data.MetadataDivergence] = strconv.FormatFloat(out.Residual, 'f', -1, 64)

							if out.VarianceDefined {
								m.Metadata[data.MetadataNoiseVariance] = strconv.FormatFloat(out.Variance, 'f', -1, 64)
							}
						}
					},
				),
			),
			// 2. Finalize
			data.NewFinalizer[float64](),
		),
	}

	level3.System = runtime.NewSystem(ctx, "morphology:level3", level3)
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
	m := data.NewMeasurement[float64]("morphology:level3", map[string]data.Metric[float64]{
		"book_shape_distance":        data.NewMetric[float64]("book_shape_distance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"book_shape_ks":              data.NewMetric[float64]("book_shape_ks", data.UnitDimensionless, data.TimescaleInstantaneous, 0.5, 0.5),
		"concentration:bid":          data.NewMetric[float64]("concentration:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"concentration:ask":          data.NewMetric[float64]("concentration:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"entropy:bid":                data.NewMetric[float64]("entropy:bid", data.UnitNat, data.TimescaleInstantaneous, 0, 0),
		"entropy:ask":                data.NewMetric[float64]("entropy:ask", data.UnitNat, data.TimescaleInstantaneous, 0, 0),
		"morphology_change":          data.NewMetric[float64]("morphology_change", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"morphology_change_baseline": data.NewMetric[float64]("morphology_change_baseline", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 0),
		"morphology_change_zscore":   data.NewMetric[float64]("morphology_change_zscore", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1),
	})
	m.Metadata["peer-interest"] = "*"
	return m
}
