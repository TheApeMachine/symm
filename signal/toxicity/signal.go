package toxicity

import (
	"context"
	"math"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type symbolState struct {
	hasPrev           bool
	prevBid           float64
	prevBidQty        float64
	prevAtNano        float64
	pendingTradeQty   float64
	pendingMatchedBid float64
}

type Signal struct {
	*runtime.System
	books  *broker.Book
	states map[string]*symbolState
	keys   []string
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: make(map[string]*symbolState),
		keys: []string{
			"retreat_fraction:bid",
			"net_withdrawal_rate:bid",
		},
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)
	return signal
}

/*
Step processes every trade entry of the tick's frame; per-symbol state is keyed
by the entry's symbol and each entry writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	for _, trade := range trades {
		signal.step(value, trade)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty
	side := trade.Side

	sideIndicator := 0.0

	switch side {
	case "buy":
		sideIndicator = 1.0
	case "sell":
		sideIndicator = -1.0
	}

	atNano := float64(value.Timestamp)

	state, exists := signal.states[trade.Symbol]

	if !exists {
		state = &symbolState{}
		signal.states[trade.Symbol] = state
	}

	// The `books.Book` callback is synchronous; the book is present only if
	// the callback ran. Pending (no snapshot yet), stale (L3 socket down) and
	// invalid (venue checksum mismatch / absent level) symbols skip the
	// callback and the tick carries book_error/<symbol> instead.
	signal.books.Book(trade.Symbol, func(b *spotbook.Book) {
		bestBid := b.BestBid()
		if bestBid == nil || bestBid.Price == nil || bestBid.Quantity == nil {
			return
		}

		bid := bestBid.Price.Float64()
		bidQty := bestBid.Quantity.Float64()

		if !state.hasPrev {
			state.observe(bid, bidQty, atNano)
			// Nothing is computable yet: the tick carries no layer for it
			// rather than zeros that read as a measured value.
			return
		}

		state.pendingTradeQty += qty

		if sideIndicator < 0 && price == state.prevBid {
			state.pendingMatchedBid += qty
		}

		if atNano <= state.prevAtNano {
			// Nothing is computable yet: the tick carries no layer for it
			// rather than zeros that read as a measured value.
			return
		}

		dt := (atNano - state.prevAtNano) / 1e9
		rated := core.Resolvable(dt)

		fill := math.Min(state.pendingMatchedBid, state.prevBidQty)
		unfilled := state.prevBidQty - fill

		var retreatFraction, withdrawalRate float64

		if bid == state.prevBid && rated {
			withdrawn := math.Max(unfilled-bidQty, 0)
			withdrawalRate = withdrawn / dt
		}

		if bid < state.prevBid && state.prevBidQty > 0 {
			retreatFraction = unfilled / state.prevBidQty
		}

		state.observe(bid, bidQty, atNano)

		keys := []string{
			signal.Name(),
			trade.Symbol,
		}

		value.Append([]float64{
			retreatFraction,
			withdrawalRate,
		}, append(keys, signal.keys...)...)
	})

	return value
}

func (state *symbolState) observe(bid, bidQty, atNano float64) {
	state.hasPrev = true
	state.prevBid = bid
	state.prevBidQty = bidQty
	state.prevAtNano = atNano
	state.pendingTradeQty = 0
	state.pendingMatchedBid = 0
}
