package morphology

import (
	"context"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

var outputKeys = []string{
	"book_shape_distance",
	"book_shape_ks",
	"concentration:bid",
	"concentration:ask",
	"entropy:bid",
	"entropy:ask",
	"morphology_change",
}

type Signal struct {
	*runtime.System
	books    broker.BookSource
	pipeline core.Primitive
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore",
				store.NewKV(),
				nomagique.NewNumber(
					// Stage 1: Select inputs for the 6 distribution primitives
					// 0: pairs -> Wasserstein1
					// 1: pairs -> KolmogorovSmirnov
					// 2: bids  -> ConcentrationPoints (bid)
					// 3: asks  -> ConcentrationPoints (ask)
					// 4: bids  -> EntropyPoints (bid)
					// 5: asks  -> EntropyPoints (ask)
					data.NewSelect(0, 0, 1, 2, 1, 2),
					data.NewBatch(1, 1, 1, 1, 1, 1),
					transport.NewParallel(
						distribution.NewWasserstein1Pairs(),
						distribution.NewKolmogorovSmirnovPairs(),
						distribution.NewConcentrationPoints(),
						distribution.NewConcentrationPoints(),
						distribution.NewEntropyPoints(),
						distribution.NewEntropyPoints(),
					),
					// Stage 2: Select 0..5 and repeat distance at 6, then pass 0..5 and compute morphology_change at 6
					data.NewSelect(0, 1, 2, 3, 4, 5, 0),
					data.NewBatch(1, 1, 1, 1, 1, 1, 1),
					transport.NewParallel(
						transport.NewPass(),
						transport.NewPass(),
						transport.NewPass(),
						transport.NewPass(),
						transport.NewPass(),
						transport.NewPass(),
						nomagique.NewNumber(
							temporal.NewDelta(),
							calculus.NewAbsolute(),
						),
					),
				),
			),
		),
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		return nil
	}

	if signal.books == nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", nil))
		return nil
	}

	var bids, asks [][2]float64
	var crossedBid, crossedAsk float64
	ok := false
	crossed := false

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}

		bid := book.BestBid()
		ask := book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		bidPrice := kraken.Float64(bid.Price)
		askPrice := kraken.Float64(ask.Price)

		if askPrice <= bidPrice {
			crossed = true
			crossedBid = bidPrice
			crossedAsk = askPrice
			return
		}

		spread := askPrice - bidPrice
		mid := (askPrice + bidPrice) / 2.0

		for cursor, count := bid, 0; cursor != nil && count < 100; cursor, count = cursor.Lower, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			p := kraken.Float64(cursor.Price)
			q := kraken.Float64(cursor.Quantity)

			if p <= 0 || q <= 0 {
				continue
			}

			rBid := (mid - p) / spread
			w := p * q
			bids = append(bids, [2]float64{rBid, w})
		}

		for cursor, count := ask, 0; cursor != nil && count < 100; cursor, count = cursor.Higher, count+1 {
			if cursor.Price == nil || cursor.Quantity == nil {
				continue
			}

			p := kraken.Float64(cursor.Price)
			q := kraken.Float64(cursor.Quantity)

			if p <= 0 || q <= 0 {
				continue
			}

			rAsk := (p - mid) / spread
			w := p * q
			asks = append(asks, [2]float64{rAsk, w})
		}

		ok = len(bids) > 0 && len(asks) > 0
	})

	if crossed {
		signal.Error(broker.CrossedTouch("morphology", prior.Label, crossedBid, crossedAsk))
		return nil
	}

	if !ok {
		return nil
	}

	pairs := [2][][2]float64{bids, asks}

	output := make(map[string]float64)
	index := 0

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewPointers(
				unsafe.Pointer(&pairs),
				unsafe.Pointer(&bids),
				unsafe.Pointer(&asks),
			),
		).Next(nil),
	) {
		if index >= len(outputKeys) {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[signal.morphology] overflow",
				nil,
			))
			return nil
		}

		if ptr == nil {
			continue
		}

		output[outputKeys[index]] = *(*float64)(ptr)
		index++
	}

	return prior.Next(signal.Name(), output)
}
