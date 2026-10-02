package morphology

import (
	"errors"
	"iter"
	"math"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
ShapeFlow measures the geometric shape of an order book by folding bids and asks
onto a dimensionless distance-from-midpoint axis and evaluating Wasserstein-1,
Kolmogorov-Smirnov, concentration, entropy, and structural shape change.
*/
type ShapeFlow struct {
	err          error
	books        broker.BookSource
	hasPrev      bool
	prevDistance float64
	w1           core.Primitive
	ks           core.Primitive
	concBid      core.Primitive
	concAsk      core.Primitive
	entBid       core.Primitive
	entAsk       core.Primitive
}

func NewShapeFlow(books broker.BookSource) core.Primitive {
	return &ShapeFlow{
		books:   books,
		w1:      distribution.NewWasserstein1Pairs(),
		ks:      distribution.NewKolmogorovSmirnovPairs(),
		concBid: distribution.NewConcentrationPoints(),
		concAsk: distribution.NewConcentrationPoints(),
		entBid:  distribution.NewEntropyPoints(),
		entAsk:  distribution.NewEntropyPoints(),
	}
}

func (op *ShapeFlow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			measurement := *(**data.Measurement[float64])(arriving)

			if measurement == nil || measurement.Err != nil {
				if !yield(arriving) {
					return
				}

				continue
			}

			distance := measurement.GetMetric("book_shape_distance").Raw
			ks := measurement.GetMetric("book_shape_ks").Raw
			concBid := measurement.GetMetric("concentration:bid").Raw
			concAsk := measurement.GetMetric("concentration:ask").Raw
			entBid := measurement.GetMetric("entropy:bid").Raw
			entAsk := measurement.GetMetric("entropy:ask").Raw

			if distance == 0 && op.books != nil {
				var bidPoints []distribution.WeightedPoint
				var askPoints []distribution.WeightedPoint

				var bidPrice, askPrice float64

				op.books.Book(measurement.Label, func(b *spotbook.Book) {
					if bid := b.BestBid(); bid != nil && bid.Price != nil {
						bidPrice = bid.Price.Float64()
					}
					if ask := b.BestAsk(); ask != nil && ask.Price != nil {
						askPrice = ask.Price.Float64()
					}

					mid := (bidPrice + askPrice) / 2.0
					spread := askPrice - bidPrice

					if spread > 0 {
						cursor := b.BestBid()
						for count := 0; count < 100 && cursor != nil; count++ {
							price := cursor.Price.Float64()
							qty := cursor.Quantity.Float64()
							relativePos := (mid - price) / spread
							bidPoints = append(bidPoints, distribution.WeightedPoint{
								Position: relativePos,
								Weight:   price * qty,
							})
							cursor = cursor.Lower
						}

						cursor = b.BestAsk()
						for count := 0; count < 100 && cursor != nil; count++ {
							price := cursor.Price.Float64()
							qty := cursor.Quantity.Float64()
							relativePos := (price - mid) / spread
							askPoints = append(askPoints, distribution.WeightedPoint{
								Position: relativePos,
								Weight:   price * qty,
							})
							cursor = cursor.Higher
						}
					}
				})

				if len(bidPoints) > 0 && len(askPoints) > 0 {
					pairs := distribution.PairsInput{Left: bidPoints, Right: askPoints}

					for out := range op.w1.Next(transport.NewOne(unsafe.Pointer(&pairs)).Next(nil)) {
						distance = *(*float64)(out)
					}

					for out := range op.ks.Next(transport.NewOne(unsafe.Pointer(&pairs)).Next(nil)) {
						ks = *(*float64)(out)
					}

					bidInput := distribution.PointsInput{Points: bidPoints}
					for out := range op.concBid.Next(transport.NewOne(unsafe.Pointer(&bidInput)).Next(nil)) {
						concBid = *(*float64)(out)
					}
					for out := range op.entBid.Next(transport.NewOne(unsafe.Pointer(&bidInput)).Next(nil)) {
						entBid = *(*float64)(out)
					}

					askInput := distribution.PointsInput{Points: askPoints}
					for out := range op.concAsk.Next(transport.NewOne(unsafe.Pointer(&askInput)).Next(nil)) {
						concAsk = *(*float64)(out)
					}
					for out := range op.entAsk.Next(transport.NewOne(unsafe.Pointer(&askInput)).Next(nil)) {
						entAsk = *(*float64)(out)
					}
				}
			}

			hasShape := false
			if distance > 0 || ks > 0 || concBid > 0 || concAsk > 0 {
				hasShape = true
			}

			if hasShape {
				measurement.WriteMetric("book_shape_distance", distance)
				measurement.WriteNormalized("book_shape_ks", ks)
				measurement.WriteNormalized("concentration:bid", concBid)
				measurement.WriteNormalized("concentration:ask", concAsk)
				measurement.WriteMetric("entropy:bid", entBid)
				measurement.WriteMetric("entropy:ask", entAsk)

				if op.hasPrev {
					change := math.Abs(distance - op.prevDistance)
					measurement.WriteMetric("morphology_change", change)
				}

				op.prevDistance = distance
				op.hasPrev = true
			}

			measurement.EnsureMetadata()

			if !yield(arriving) {
				return
			}
		}
	}
}

func (op *ShapeFlow) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}
