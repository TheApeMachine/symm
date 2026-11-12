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

/*
Signal is the Pumpdump disruptor stage — outcomes land on envelope.Pumpdump and
Map() feeds logic/resonance Features (compression, exhaustion) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books *broker.Book
	algo  *equation.Ignition
}

/*
NewSignal binds L3 touch state on each trade tick — Pumpdump labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System: runtime.NewSystem(ctx, "pumpdump"),
		books:  books,
		algo:   equation.NewIgnition(128),
	}
}

/*
Step fans one trade frame in parallel — metrics aggregate later, but each symbol
needs fresh readings here or featureScorer never marks that Feature as seen.
Ignition uses touch bid/ask with print for enter/exit compression features.
*/
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
