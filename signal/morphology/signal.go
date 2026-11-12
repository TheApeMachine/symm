package morphology

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Morphology disruptor stage — outcomes land on envelope.Morphology and
Map() feeds logic/resonance Features (ask_vacuum) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.MorphologySample
	process *equation.Morphology
}

/*
NewSignal binds L3 touch state on each trade tick — Morphology labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "morphology"),
		books:   books,
		sample:  algorithm.NewMorphologySample(),
		process: equation.NewMorphology(),
	}
}

/*
Step fans one trade frame in parallel — metrics aggregate later, but each symbol
needs fresh readings here or featureScorer never marks that Feature as seen.

*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
			var (
				bids     []flow.BookLevel
				asks     []flow.BookLevel
				haveBook bool
			)

			signal.books.Book(data.Symbol, func(book *market.Book) {
				bids = make([]flow.BookLevel, 0, len(book.Bids.Levels))
				for _, level := range book.Bids.Levels {
					bids = append(bids, flow.BookLevel{
						Price:    level.Price.InexactFloat64(),
						Quantity: level.Quantity.InexactFloat64(),
					})
				}

				asks = make([]flow.BookLevel, 0, len(book.Asks.Levels))
				for _, level := range book.Asks.Levels {
					asks = append(asks, flow.BookLevel{
						Price:    level.Price.InexactFloat64(),
						Quantity: level.Quantity.InexactFloat64(),
					})
				}

				haveBook = true
			})

			if !haveBook {
				return
			}

			input, ready, err := signal.sample.MeasureBook(flow.BookInput{
				Symbol: data.Symbol,
				Bids:   bids,
				Asks:   asks,
			})

			if err != nil {
				signal.Error(err)
				return
			}

			if !ready {
				return
			}

			outcome, err := signal.process.Measure(input)
			if err != nil {
				signal.Error(err)
				return
			}

			mu.Lock()
			envelope.Morphology[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Morphology[focus]; ok {
		ui.Focus("morphology", focus, reading)
	}

	return envelope
}
