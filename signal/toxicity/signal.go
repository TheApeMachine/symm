package toxicity

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/algorithm/book/quality"
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
	sample  *quality.Sample
	process *equation.BookQuality
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "toxicity"),
		books:   books,
		sample:  quality.NewSample(),
		process: equation.NewBookQuality(),
	}
}

/*
Step processes every trade entry of the tick's frame; book and trade evidence
feed BookQuality, and each ready outcome lands on the envelope.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		task := func() {
			defer wg.Done()

			if data.Price.IsZero() || data.Qty.IsZero() {
				return
			}

			price := data.Price.InexactFloat64()
			qty := data.Qty.InexactFloat64()

			if price <= 0 || qty <= 0 {
				return
			}

			var (
				input equation.BookQualityInput
				ready bool
				err   error
			)

			signal.books.Book(data.Symbol, func(book *market.Book) {
				bids := make([]flow.BookLevel, 0, len(book.Bids.Levels))
				for _, level := range book.Bids.Levels {
					bids = append(bids, flow.BookLevel{
						Price:    level.Price.InexactFloat64(),
						Quantity: level.Quantity.InexactFloat64(),
					})
				}

				asks := make([]flow.BookLevel, 0, len(book.Asks.Levels))
				for _, level := range book.Asks.Levels {
					asks = append(asks, flow.BookLevel{
						Price:    level.Price.InexactFloat64(),
						Quantity: level.Quantity.InexactFloat64(),
					})
				}

				input, ready, _, err = signal.sample.MeasureBook(flow.BookInput{
					Symbol: data.Symbol,
					Bids:   bids,
					Asks:   asks,
				})

				if err != nil || !ready {
					return
				}

				input, ready, _, err = signal.sample.MeasureTrade(flow.TradeInput{
					Symbol:   data.Symbol,
					Price:    price,
					Quantity: qty,
					Side:     flow.TradeSide(data.Side.String()),
					At:       data.Timestamp,
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

			if outcome.Category == 0 {
				return
			}

			mu.Lock()
			envelope.Toxicity[data.Symbol] = &outcome
			mu.Unlock()
		}

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Toxicity[focus]; ok {
		ui.Focus("toxicity", focus, reading)
	}

	return envelope
}
