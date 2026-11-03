package pumpdump

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		task := func() {
			defer wg.Done()

			signal.books.Book(data.Symbol, func(book *market.Book) {
				price := data.Price.InexactFloat64()
				qty := data.Qty.InexactFloat64()

				output, ready, _, err := signal.algo.Measure(equation.IgnitionInput{
					At:     data.Timestamp,
					Symbol: data.Symbol,
					Last:   price,
					Volume: qty,
					Ask:    book.BestAsk().Price.InexactFloat64(),
					Bid:    book.BestBid().Price.InexactFloat64(),
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

		if system.Q.Submit(task) != nil {
			task()
		}
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Pumpdump[focus]; ok {
		ui.Focus("pumpdump", focus, reading)
	}

	return envelope
}
