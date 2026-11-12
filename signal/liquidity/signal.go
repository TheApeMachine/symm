package liquidity

import (
	"context"
	"sync"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Signal is the Liquidity disruptor stage — outcomes land on envelope.Liquidity and
Map() feeds logic/resonance Features (scarcity_score, depth_score) for Model story gates.
*/
type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.DepthSample
	process *equation.Depth
}

/*
NewSignal binds L3 touch state on each trade tick — Liquidity labels are book-native,
not printable from public tape alone.
*/
func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "liquidity"),
		books:   books,
		sample:  algorithm.NewDepthSample(),
		process: equation.NewDepth(),
	}
}

/*
Step fans one trade frame in parallel — metrics aggregate later, but each symbol
needs fresh readings here or featureScorer never marks that Feature as seen.

*/
func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Go(func() {
			var (
				bid, ask, bidQty, askQty float64
				ok                       bool
			)

			signal.books.Book(data.Symbol, func(book *market.Book) {
				bestBid := book.BestBid()
				bestAsk := book.BestAsk()

				if bestBid == nil || bestAsk == nil ||
					bestBid.Price == nil || bestAsk.Price == nil ||
					bestBid.Quantity == nil || bestAsk.Quantity == nil {
					return
				}

				bid = bestBid.Price.InexactFloat64()
				ask = bestAsk.Price.InexactFloat64()
				bidQty = bestBid.Quantity.InexactFloat64()
				askQty = bestAsk.Quantity.InexactFloat64()
				ok = true
			})

			if !ok {
				return
			}

			input, ready, err := signal.sample.Measure(algorithm.DepthSampleInput{
				Symbol: data.Symbol,
				Bid:    bid,
				Ask:    ask,
				BidQty: bidQty,
				AskQty: askQty,
				At:     data.Timestamp,
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
			envelope.Liquidity[data.Symbol] = &outcome
			mu.Unlock()
		})
	}

	wg.Wait()

	focus := types.Focus()
	if reading, ok := envelope.Liquidity[focus]; ok {
		ui.Focus("liquidity", focus, reading)
	}

	return envelope
}
