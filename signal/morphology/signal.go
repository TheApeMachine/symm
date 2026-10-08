package morphology

import (
	"context"
	"strconv"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

var outputKeys = []string{
	"book_shape_distance", "book_shape_ks",
	"concentration:bid", "concentration:ask", "entropy:bid", "entropy:ask",
	"morphology_change",
}

type Signal struct {
	*runtime.System
	books    broker.BookSource
	pipeline *nomagique.Number
}

func NewSignal(ctx context.Context, books broker.BookSource) *Signal {
	signal := &Signal{
		books: books,
		pipeline: nomagique.NewNumber(
			transport.NewAddressable("symbolstore", store.NewKV(func() core.Primitive {
				// Both sides use the same stateless projection. Parallel consumes
				// branches in deterministic order; only Previous retains a book.
				projection := nomagique.NewNumber(
					// Input: bid, ask, then individual price/quantity operands.
					transport.NewFanout[float64](
						nomagique.NewNumber(data.NewSelect(0, 1), calculus.NewPositive(), transport.NewDiscard()),
						nomagique.NewNumber(data.NewSelect(0, 1), arithmetic.NewAdd(), vector.NewScale(0.5)),
						nomagique.NewNumber(data.NewSelect(1, 0), arithmetic.NewSubtract(), calculus.NewPositive()),
						data.NewSlice(2),
					),
					// Per level: signed spread coordinate and displayed notional.
					transport.NewMap(2, transport.NewFanout[float64](
						nomagique.NewNumber(
							transport.NewFanout[float64](
								nomagique.NewNumber(data.NewSelect(2, 0), arithmetic.NewSubtract()),
								data.NewSelect(1),
							), arithmetic.NewDivide(),
						),
						nomagique.NewNumber(
							transport.NewFanout[float64](
								nomagique.NewNumber(data.NewSelect(2), calculus.NewPositive()),
								data.NewSelect(3),
							), vector.NewScale(),
						),
					), 2),
					distribution.NewSortedPositions(),
					distribution.NewNormalize(2, 1),
					data.NewPack[float64](),
				)

				return nomagique.NewNumber(
					transport.NewParallel(projection, projection),
					// A whole book requires both normalized side distributions.
					data.NewPack[core.Primitive](2), data.NewUnpack(),
					transport.NewFanout[core.Primitive](
						// Fold each normalized side only for bilateral comparisons.
						nomagique.NewNumber(
							transport.NewParallel(
								nomagique.NewNumber(
									transport.NewMap(2, transport.NewFanout[float64](
										nomagique.NewNumber(data.NewSelect(0), calculus.NewAbsolute()),
										data.NewSelect(1),
									)), distribution.NewSortedPositions(), data.NewPack[float64](),
								),
								data.NewPack[float64](),
							), distribution.NewMergedWalk(), data.NewSelect(1, 0),
						),
						// Moments use the same side masses, not recomputed price data.
						nomagique.NewNumber(
							data.NewSelect(0, 1, 0, 1),
							transport.NewParallel(
								distribution.NewConcentrationPoints(), distribution.NewConcentrationPoints(),
								distribution.NewEntropyPoints(), distribution.NewEntropyPoints(),
							),
						),
						// Keep signed positions. Each side contributes half the mass.
						nomagique.NewNumber(
							data.NewUnpack(),
							transport.NewMap(2, transport.NewFanout[float64](
								data.NewSelect(0),
								nomagique.NewNumber(data.NewSelect(1), vector.NewScale(0.5)),
							)), temporal.NewPrevious(), distribution.NewWasserstein1Pairs(),
						),
					),
				)
			})),
		),
	}

	signal.System = runtime.NewSystem(ctx, "morphology", signal)

	if err := errnie.Require(map[string]any{"books": books}); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] book manager is required", err))
	}

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	var bids, asks []float64
	var inputErr error

	signal.books.Book(prior.Label, func(book *spotbook.Book) {
		if book == nil {
			return
		}
		bid, ask := book.BestBid(), book.BestAsk()

		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}
		bids = append(bids, kraken.Float64(bid.Price), kraken.Float64(ask.Price))
		asks = append(asks, kraken.Float64(bid.Price), kraken.Float64(ask.Price))

		for cursor := bid; cursor != nil; cursor = cursor.Lower {
			if cursor.Price == nil || cursor.Quantity == nil {
				inputErr = core.ErrShape
				return
			}

			bids = append(bids, kraken.Float64(cursor.Price), kraken.Float64(cursor.Quantity))
		}

		for cursor := ask; cursor != nil; cursor = cursor.Higher {
			if cursor.Price == nil || cursor.Quantity == nil {
				inputErr = core.ErrShape
				return
			}

			asks = append(asks, kraken.Float64(cursor.Price), kraken.Float64(cursor.Quantity))
		}
	})

	if inputErr != nil {
		signal.Error(errnie.Err(errnie.Validation, "[morphology] incomplete book level", inputErr))
		return nil
	}

	if len(bids) == 0 || len(asks) == 0 {
		return nil
	}

	output := make(map[string]float64)
	index := 0

	for pointer := range signal.pipeline.Next(data.NewMessage(
		data.EVALUATE, "symbolstore", strconv.FormatInt(prior.Epoch, 10)+"/"+prior.Label,
		data.NewValue[core.Primitive](data.NewValue(bids...), data.NewValue(asks...)),
	).Next(nil)) {
		if pointer == nil || index >= len(outputKeys) {
			signal.Error(errnie.Err(errnie.Validation, "[morphology] invalid pipeline output", core.ErrShape))
			return nil
		}

		output[outputKeys[index]] = *(*float64)(pointer)
		index++
	}

	if err := signal.pipeline.Error(); err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[morphology] "+prior.Label+": invalid book or pipeline state", err))
		return nil
	}

	// Only the final metric is optional: a first book has no prior comparison.
	if index != len(outputKeys)-1 && index != len(outputKeys) {
		return nil
	}

	return prior.Next(signal.Name(), output)
}
