package liquidity

import (
	"context"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	parent "github.com/theapemachine/symm/signal"
)

type Signal struct {
	*runtime.System
	books *broker.Book
	keys  []string
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books: books,
		keys: []string{
			"best_price:bid",
			"best_price:ask",
			"relative_spread",
			"fee_rate",
			"friction_hurdle",
		},
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)
	return signal
}

/*
Step processes the book of every distinct symbol traded in the tick's frame;
each symbol writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	symbols := parent.DistinctSymbols(trades)

	if err := parent.RunSymbols(signal.Context(), symbols, func(symbol string) {
		signal.step(value, symbol)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, symbol string) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	var bookAvailable bool
	signal.books.Book(symbol, func(b *spotbook.Book) {
		bookAvailable = true

		bestBid := b.BestBid()
		bestAsk := b.BestAsk()
		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bid := bestBid.Price.Float64()
		ask := bestAsk.Price.Float64()

		midpoint := (bid + ask) / 2.0
		var relSpread float64
		if midpoint > 0 {
			relSpread = (ask - bid) / midpoint
		}

		feeRate := 0.0
		var frictionHurdle float64
		if feeRate > 0 {
			frictionHurdle = relSpread + 2.0*feeRate
		}

		keys := []string{
			signal.Name(),
			symbol,
		}

		value.Append([]float64{
			bid,
			ask,
			relSpread,
			feeRate,
			frictionHurdle,
		}, append(keys, signal.keys...)...)
	})

	if !bookAvailable {
		return value
	}

	return value
}
