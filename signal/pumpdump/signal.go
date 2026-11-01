package pumpdump

import (
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/signal/frame"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	books *broker.Book
	algo  *equation.Ignition
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System: runtime.NewSystem(ctx, "pumpdump"),
		books:  books,
		algo:   equation.NewIgnition(128),
	}
}

func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var mu sync.Mutex

	frame.NewSymbols(envelope.Trade.Data).Each(func(trades []*market.TradeData) {
		for _, data := range trades {
			signal.books.Book(data.Symbol, func(book *book.Book) {
				price, _ := data.Price.Float64()
				qty, _ := data.Qty.Float64()

				output, ready, _, err := signal.algo.Measure(equation.IgnitionInput{
					At:     data.Timestamp,
					Symbol: data.Symbol,
					Last:   price,
					Volume: qty,
					Ask:    book.BestAsk().Price.Float64(),
					Bid:    book.BestBid().Price.Float64(),
				})

				if err != nil {
					signal.Error(err)
					return
				}

				if !ready {
					return
				}

				mu.Lock()
				envelope.Pumpdump[data.Symbol] = &output
				mu.Unlock()
			})
		}
	})

	ui.UI.Write(envelope.Publish("pumpdump"))
	return envelope
}
