package toxicity

import (
	"context"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/algorithm/book/quality"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/frame"
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
	var mu sync.Mutex

	frame.NewSymbols(envelope.Trade.Data).Each(func(trades []*market.TradeData) {
		for _, data := range trades {
			if data.Price.IsZero() || data.Qty.IsZero() {
				continue
			}

			price, _ := data.Price.Float64()
			qty, _ := data.Qty.Float64()

			if price <= 0 || qty <= 0 {
				continue
			}

			var (
				input equation.BookQualityInput
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
				continue
			}

			if !ready {
				continue
			}

			outcome, err := signal.process.Measure(input)
			if err != nil {
				signal.Error(err)
				continue
			}

			if outcome.Category == 0 {
				continue
			}

			mu.Lock()
			envelope.Toxicity[data.Symbol] = &outcome
			mu.Unlock()
		}
	})

	ui.UI.Write(envelope.Publish("toxicity"))
	return envelope
}
