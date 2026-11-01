package liquidity

import (
	"context"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

type Signal struct {
	*runtime.System
	books   *broker.Book
	sample  *algorithm.DepthSample
	process *equation.Depth
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	return &Signal{
		System:  runtime.NewSystem(ctx, "liquidity"),
		books:   books,
		sample:  algorithm.NewDepthSample(),
		process: equation.NewDepth(),
	}
}

func (signal *Signal) Step(envelope *runtime.Envelope) *runtime.Envelope {
	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	for _, data := range envelope.Trade.Data {
		wg.Add(1)

		system.Q.Submit(func() {
			defer wg.Done()

			var (
				bid, ask, bidQty, askQty float64
				ok                       bool
			)

			signal.books.Book(data.Symbol, func(book *spotbook.Book) {
				bestBid := book.BestBid()
				bestAsk := book.BestAsk()

				if bestBid == nil || bestAsk == nil ||
					bestBid.Price == nil || bestAsk.Price == nil ||
					bestBid.Quantity == nil || bestAsk.Quantity == nil {
					return
				}

				bid = bestBid.Price.Float64()
				ask = bestAsk.Price.Float64()
				bidQty = bestBid.Quantity.Float64()
				askQty = bestAsk.Quantity.Float64()
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

			if err != nil || !ready {
				signal.Error(err)
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

	ui.UI.Write(envelope.Publish("liquidity"))
	return envelope
}
