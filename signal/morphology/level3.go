package morphology

import (
	"context"
	"math"
	"strconv"
	"sync"
	"unsafe"

	"github.com/theapemachine/errnie"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
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
		w1           core.Primitive
		ks           core.Primitive
		concBid      core.Primitive
		concAsk      core.Primitive
		entBid       core.Primitive
		entAsk       core.Primitive
	}
	state := &morphologyState{
		w1:      distribution.NewWasserstein1Pairs(),
		ks:      distribution.NewKolmogorovSmirnovPairs(),
		concBid: distribution.NewConcentrationPoints(),
		concAsk: distribution.NewConcentrationPoints(),
		entBid:  distribution.NewEntropyPoints(),
		entAsk:  distribution.NewEntropyPoints(),
	}

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

				if distance == 0 && level3.books != nil {
					var bidPoints []distribution.WeightedPoint
					var askPoints []distribution.WeightedPoint
					
					var bidPrice, askPrice float64
					
					level3.books.Book(m.Label, func(b *spotbook.Book) {
						if bid := b.BestBid(); bid != nil && bid.Price != nil {
							bidPrice = bid.Price.Float64()
						}
						if ask := b.BestAsk(); ask != nil && ask.Price != nil {
							askPrice = ask.Price.Float64()
						}
						
						mid := (bidPrice + askPrice) / 2.0
						spread := askPrice - bidPrice
						
						if spread > 0 {
							// Bid points (folded position: (mid - p) / spread)
							cursor := b.BestBid()
							for count := 0; count < 100 && cursor != nil; count++ {
								p := cursor.Price.Float64()
								q := cursor.Quantity.Float64()
								r := (mid - p) / spread
								bidPoints = append(bidPoints, distribution.WeightedPoint{Position: r, Weight: p * q})
								cursor = cursor.Lower
							}
							
							// Ask points (folded position: (p - mid) / spread)
							cursor = b.BestAsk()
							for count := 0; count < 100 && cursor != nil; count++ {
								p := cursor.Price.Float64()
								q := cursor.Quantity.Float64()
								r := (p - mid) / spread
								askPoints = append(askPoints, distribution.WeightedPoint{Position: r, Weight: p * q})
								cursor = cursor.Higher
							}
						}
					})
					
					if len(bidPoints) > 0 && len(askPoints) > 0 {
						// Since we appended by traversing the book downwards for bids and upwards for asks,
						// the folded distance r is naturally ascending! Both (mid-p)/spread and (p-mid)/spread increase.
						
						pairs := distribution.PairsInput{Left: bidPoints, Right: askPoints}
						
						for out := range state.w1.Next(transport.NewOne(unsafe.Pointer(&pairs)).Next(nil)) {
							distance = *(*float64)(out)
						}
						
						for out := range state.ks.Next(transport.NewOne(unsafe.Pointer(&pairs)).Next(nil)) {
							ks = *(*float64)(out)
						}
						
						bidIn := distribution.PointsInput{Points: bidPoints}
						for out := range state.concBid.Next(transport.NewOne(unsafe.Pointer(&bidIn)).Next(nil)) {
							concBid = *(*float64)(out)
						}
						for out := range state.entBid.Next(transport.NewOne(unsafe.Pointer(&bidIn)).Next(nil)) {
							entBid = *(*float64)(out)
						}
						
						askIn := distribution.PointsInput{Points: askPoints}
						for out := range state.concAsk.Next(transport.NewOne(unsafe.Pointer(&askIn)).Next(nil)) {
							concAsk = *(*float64)(out)
						}
						for out := range state.entAsk.Next(transport.NewOne(unsafe.Pointer(&askIn)).Next(nil)) {
							entAsk = *(*float64)(out)
						}
					}
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
						m.WriteStandardized("morphology_change_zscore", out.ZScore)
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

func (level3 *Level3) Step(measurement *data.Measurement[float64]) *data.Measurement[float64] {
	if level3.Status() != runtime.READY {
		errnie.Warn(level3.Name() + ": Step called before READY; dropping event")
		return measurement
	}

	if measurement == nil || measurement.Err != nil {
		return measurement
	}

	measurement.SetSource("morphology:level3")

	return data.Read[*data.Measurement[float64]](level3.pipelineFor(measurement.Label).Next(
		transport.NewOne(unsafe.Pointer(&measurement)).Next(nil),
	))
}
