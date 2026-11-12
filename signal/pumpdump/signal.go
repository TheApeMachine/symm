package pumpdump

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Pumpdump disruptor stage — outcomes land on envelope.Pumpdump and
Map() feeds logic/resonance Features (compression, exhaustion) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books  *broker.Book
	price  *broker.Price
	algo   *equation.Ignition
	pulse  atomic.Uint64
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
Step fans trade prints and book heartbeats so quiet symbols still warm ignition.
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
			signal.Error(err)
			return
		}

		if !ready {
			return
		}

		mu.Lock()
		envelope.Pumpdump[symbol] = &output
		mu.Unlock()
	}

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

			measure(
				data.Symbol, data.Timestamp,
				data.Price.InexactFloat64(), bid, ask,
				data.Qty.InexactFloat64(),
			)
		})
	}

	if signal.price != nil {
		batch := system.Cfg.Selection.HeartbeatBatch

		if batch <= 0 {
			batch = 48
		}

		start := int(signal.pulse.Load())
		symbols, next := signal.price.PulseSymbols(batch, start)

		for _, symbol := range symbols {
			last, bid, ask, ok := signal.price.PulseQuote(symbol)

			if !ok {
				continue
			}

			symbol, last, bid, ask := symbol, last, bid, ask

			wg.Go(func() {
				measure(symbol, time.Now(), last, bid, ask, 1e-12)
			})
		}

		signal.pulse.Store(uint64(next))
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Pumpdump[focus]; ok {
		ui.Focus("pumpdump", focus, reading)
	}

	return envelope
}
