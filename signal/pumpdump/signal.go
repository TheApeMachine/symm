package pumpdump

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
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
	price *broker.Price
	algo  *equation.Ignition
}

/*
NewSignal binds L3 touch state on each trade tick — Pumpdump labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book, price *broker.Price) *Signal {
	return &Signal{
		System: runtime.NewSystem(ctx, "pumpdump"),
		books:  books,
		price:  price,
		algo:   equation.NewIgnition(128),
	}
}

/*
Step scores each symbol's prints in arrival order on the volume clock.
Heartbeats are not prints and do not advance the clock.
*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	measure := func(symbol string, at time.Time, last, bid, ask, vol float64) {
		if symbol == "" || vol <= 0 {
			return
		}

		output, ready, _, err := signal.algo.Measure(equation.IgnitionInput{
			At:     at,
			Symbol: symbol,
			Last:   last,
			Volume: vol,
			Ask:    ask,
			Bid:    bid,
		})

		if err != nil {
			// Transient tick validation (crossed book, etc.) must not park the
			// signal System in ERROR for the rest of the process.
			errnie.Error(err)
			return
		}

		if !ready {
			return
		}

		if signal.price != nil {
			signal.price.NoteExhaustion(symbol, output.Exhaustion)
		}

		mu.Lock()
		envelope.Pumpdump[symbol] = &output
		mu.Unlock()
	}

	batches := envelope.Trade.Batches()

	for _, batch := range batches {
		wg.Go(func() {
			for _, data := range batch {
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
					continue
				}

				measure(
					data.Symbol, data.Timestamp,
					data.Price.InexactFloat64(), bid, ask,
					data.Qty.InexactFloat64(),
				)
			}
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Pumpdump[focus]; ok {
		ui.Focus("pumpdump", focus, reading)
	}

	return envelope
}
