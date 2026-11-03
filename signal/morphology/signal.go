package morphology

import (
	"context"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.MorphologySample
	process *equation.Morphology
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "morphology"),
		books:   books,
		sample:  algorithm.NewMorphologySample(),
		process: equation.NewMorphology(),
	}
}

func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		task := func() {
			defer wg.Done()

			var (
				input equation.MorphologyInput
				ready bool
				err   error
			)

			signal.books.Book(data.Symbol, func(book *spotbook.Book) {
				bids := make([]flow.BookLevel, 0, len(book.Bids.Levels))
				for _, level := range book.Bids.Levels {
					bids = append(bids, flow.BookLevel{
						Price:    level.Price.Float64(),
						Quantity: level.Quantity.Float64(),
					})
				}

				asks := make([]flow.BookLevel, 0, len(book.Asks.Levels))
				for _, level := range book.Asks.Levels {
					asks = append(asks, flow.BookLevel{
						Price:    level.Price.Float64(),
						Quantity: level.Quantity.Float64(),
					})
				}

				input, ready, err = signal.sample.MeasureBook(flow.BookInput{
					Symbol: data.Symbol,
					Bids:   bids,
					Asks:   asks,
				})
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
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Morphology[focus]; ok {
		ui.Focus("morphology", focus, reading)
	}

	return envelope
}
