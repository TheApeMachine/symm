package depthflow

import (
	"context"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/frame"
	"github.com/theapemachine/symm/ui"
)

const bookflowHistory = 128

type Signal struct {
	*runtime.System
	books      *broker.Book
	instrument *broker.Instrument
	sample     *flow.Sample
	process    *equation.Bookflow
}

func NewSignal(
	ctx context.Context,
	books *broker.Book,
	instrument *broker.Instrument,
) *Signal {
	sample, err := flow.NewSample(bookflowHistory)

	signal := &Signal{
		System:     runtime.NewSystem(ctx, "depthflow"),
		books:      books,
		instrument: instrument,
		sample:     sample,
		process:    equation.NewBookflow(),
	}

	if err != nil {
		signal.Error(errnie.Err(
			errnie.Validation,
			"depthflow: new sample",
			err,
		))
	}

	return signal
}

/*
Step processes every trade entry of the tick's frame; book and trade pressure
feed Bookflow, and each ready outcome lands on the envelope.
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
				input equation.BookflowInput
				ready bool
				err   error
			)

			signal.books.Book(data.Symbol, func(book *spotbook.Book) {
				pair := signal.instrument.Pair(data.Symbol)

				if pair.TickSize == nil {
					return
				}

				tickSize, _ := pair.TickSize.Float64()

				if tickSize <= 0 {
					return
				}

				bids := make([]flow.BookLevel, 0, len(book.Bids.Levels))

				for _, level := range book.Bids.Levels {
					levelPrice := level.Price.Float64()
					bids = append(bids, flow.BookLevel{
						Price:    levelPrice,
						Quantity: level.Quantity.Float64(),
						Ticks:    int64(levelPrice / tickSize),
					})
				}

				asks := make([]flow.BookLevel, 0, len(book.Asks.Levels))

				for _, level := range book.Asks.Levels {
					levelPrice := level.Price.Float64()
					asks = append(asks, flow.BookLevel{
						Price:    levelPrice,
						Quantity: level.Quantity.Float64(),
						Ticks:    int64(levelPrice / tickSize),
					})
				}

				input, ready, _, err = signal.sample.MeasureBook(flow.BookInput{
					Symbol:   data.Symbol,
					TickSize: tickSize,
					Bids:     bids,
					Asks:     asks,
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

			if !outcome.Ready {
				continue
			}

			mu.Lock()
			envelope.DepthFlow[data.Symbol] = &outcome
			mu.Unlock()
		}
	})

	ui.UI.Write(envelope.Publish("depthflow"))
	return envelope
}
