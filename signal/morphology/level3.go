package morphology

import (
	"context"
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/broker"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
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
	pipelines sync.Map
	ID        int
	books     broker.BookSource
}

func NewLevel3(ctx context.Context, books broker.BookSource) *Level3 {

	level3 := &Level3{
		books: books,
	}

	level3.System = runtime.NewSystem(ctx, "morphology:level3", level3)
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

	type morphologyState struct {
		hasPrev      bool
		prevDistance float64
	}
	state := &morphologyState{}

	pipeline := nomagique.NewNumber(
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
						_, hasDist := p.LookupMetric("book_shape_distance")
						if hasDist {
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

				distance := input.GetMetric("book_shape_distance").Raw
				ks := input.GetMetric("book_shape_ks").Raw
				concBid := input.GetMetric("concentration:bid").Raw
				concAsk := input.GetMetric("concentration:ask").Raw
				entBid := input.GetMetric("entropy:bid").Raw
				entAsk := input.GetMetric("entropy:ask").Raw

				if distance == 0 {
					var bidPrice, askPrice float64
					if level3.books != nil {
						level3.books.Book(m.Label, func(b *spotbook.Book) {
							if bid := b.BestBid(); bid != nil && bid.Price != nil {
								bidPrice = bid.Price.Float64()
							}
							if ask := b.BestAsk(); ask != nil && ask.Price != nil {
								askPrice = ask.Price.Float64()
							}
						})
					}
					
					b := bidPrice
					if b == 0 {
						b = input.GetMetric("best_bid").Raw
					}
					if b == 0 {
						b = input.GetMetric("bid").Raw
					}
					
					a := askPrice
					if a == 0 {
						a = input.GetMetric("best_ask").Raw
					}
					if a == 0 {
						a = input.GetMetric("ask").Raw
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
					m.WriteMetric("book_shape_distance", distance)
					m.WriteMetric("book_shape_ks", ks)
					m.WriteMetric("concentration:bid", concBid)
					m.WriteMetric("concentration:ask", concAsk)
					m.WriteMetric("entropy:bid", entBid)
					m.WriteMetric("entropy:ask", entAsk)


					if state.hasPrev {
						change := math.Abs(distance - state.prevDistance)
						m.WriteMetric("morphology_change", change)
					}

					state.prevDistance = distance
					state.hasPrev = true
				}

				m.EnsureMetadata()

				return m
			},
			func(m *data.Measurement[float64], res *data.Measurement[float64]) {},
		),
		// 1. Baselines
		transport.NewFan(
			data.NewAdapter(
				adaptive.NewBaseline(adaptive.NewWindow()),
				func(m *data.Measurement[float64]) float64 {
					return m.GetMetric("morphology_change").Raw
				},
				func(m *data.Measurement[float64], out adaptive.BaselineReading) {
					m.SetMetadata(data.MetadataSupport, strconv.FormatFloat(out.Count, 'f', -1, 64))

					if out.HasPrior {
						m.WriteMetric("morphology_change_baseline", out.Baseline)
						m.WriteMetric("morphology_change_zscore", out.ZScore)
						m.SetMetadata(data.MetadataDivergence, strconv.FormatFloat(out.Residual, 'f', -1, 64))

						if out.VarianceDefined {
							m.SetMetadata(data.MetadataNoiseVariance, strconv.FormatFloat(out.Variance, 'f', -1, 64))
						}
					}
				},
			),
		),
		// 2. Finalize
		data.NewFinalizer[float64](),
	)

	actual, _ := level3.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}

func (level3 *Level3) Step(m *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return m
	}

	if m == nil || m.Err != nil {
		return m
	}

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(m.Label).Next(
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
	m.SetMetadata("peer-interest", "*")
	return m
}
