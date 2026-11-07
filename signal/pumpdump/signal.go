package pumpdump

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
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
		wg.Go(func() {
			var (
				bid, ask float64
				ok       bool
			)

			signal.books.Book(data.Symbol, func(book *market.Book) {
				bestBid := book.BestBid()
				bestAsk := book.BestAsk()

				if bestBid == nil || bestAsk == nil ||
					bestBid.Price == nil || bestAsk.Price == nil {
					return
				}

				bid = bestBid.Price.InexactFloat64()
				ask = bestAsk.Price.InexactFloat64()
				ok = true
			})

			if !ok {
				return
			}

			output, ready, _, err := signal.algo.Measure(equation.IgnitionInput{
				At:     data.Timestamp,
				Symbol: data.Symbol,
				Last:   data.Price.InexactFloat64(),
				Volume: data.Qty.InexactFloat64(),
				Ask:    ask,
				Bid:    bid,
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

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Pumpdump[focus]; ok {
		ui.Focus("pumpdump", focus, reading)
	}

	return envelope
}
