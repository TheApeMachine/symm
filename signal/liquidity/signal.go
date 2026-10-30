package liquidity

import (
	"context"
	"math"
	"slices"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
)

type Signal struct {
	*runtime.System
	books *broker.Book
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books: books,
	}

	signal.System = runtime.NewSystem(ctx, "liquidity", signal)
	return signal
}

/*
Step samples relative spread for every symbol on the tick, then publishes
board-wide friction and capacity oracles (no per-symbol layers).
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || !parent.HasFrame(value) {
		return value
	}

	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	symbols := parent.DistinctSymbols(trades)
	spreads := make([]float64, 0, len(symbols))

	for _, symbol := range symbols {
		if spread, ok := signal.relativeSpread(symbol); ok {
			spreads = append(spreads, spread)
		}
	}

	if len(spreads) == 0 {
		return value
	}

	slices.Sort(spreads)
	median := spreads[len(spreads)/2]
	if len(spreads)%2 == 0 {
		median = (spreads[len(spreads)/2-1] + spreads[len(spreads)/2]) / 2
	}

	worst := spreads[len(spreads)-1]

	value.Append([]float64{median}, append([]string{
		signal.Name(),
		"aggregate",
	}, "market_friction")...)

	value.Append([]float64{worst}, append([]string{
		signal.Name(),
		"aggregate",
	}, "capacity_stress")...)

	audit.Record.Write(map[string]any{
		"signal": signal.Name(),
		"median": median,
		"worst":  worst,
	})

	return value
}

func (signal *Signal) relativeSpread(symbol string) (float64, bool) {
	var spread float64
	var ok bool

	signal.books.Book(symbol, func(b *spotbook.Book) {
		bestBid := b.BestBid()
		bestAsk := b.BestAsk()
		if bestBid == nil || bestAsk == nil || bestBid.Price == nil || bestAsk.Price == nil {
			return
		}

		bid := bestBid.Price.Float64()
		ask := bestAsk.Price.Float64()
		mid := (bid + ask) / 2
		if mid <= 0 || ask <= bid {
			return
		}

		spread = (ask - bid) / mid
		ok = true
	})

	if !ok || math.IsNaN(spread) {
		return 0, false
	}

	return spread, true
}
