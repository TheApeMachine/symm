package toxicity

import (
	"context"
	"math"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
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
	states *sync.Map
	keys   []string
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: &sync.Map{},
		keys: []string{
			"spoof_pressure",
			"floor_evaporation",
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

	if err := parent.RunTrades(signal.Context(), trades, func(trade kraken.TradeData) {
		signal.step(value, trade)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, trade kraken.TradeData) *data.Value {
	errnie.Debug("[toxicity] step", "trade", trade)

	if signal.Status() != runtime.READY {
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

	var (
		retreatFraction float64
		withdrawalRate  float64
		appendMetrics   bool
	)

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

		untypedState, _ := signal.states.LoadOrStore(trade.Symbol, &symbolState{})
		state := untypedState.(*symbolState)

		if !state.hasPrev {
			state.observe(bid, bidQty, atNano)
			return
		}

		state.pendingTradeQty += qty

		if sideIndicator < 0 && price == state.prevBid {
			state.pendingMatchedBid += qty
		}

		if atNano <= state.prevAtNano {
			return
		}

		dt := (atNano - state.prevAtNano) / 1e9
		rated := dt > 0 && !math.IsInf(dt, 0)

		fill := math.Min(state.pendingMatchedBid, state.prevBidQty)
		unfilled := state.prevBidQty - fill

		if bid == state.prevBid && rated {
			withdrawn := math.Max(unfilled-bidQty, 0)
			withdrawalRate = withdrawn / dt
		}

		if bid < state.prevBid && state.prevBidQty > 0 {
			retreatFraction = unfilled / state.prevBidQty
		}

		state.observe(bid, bidQty, atNano)
		appendMetrics = true
	})

	if !appendMetrics {
		return value
	}

	keys := []string{
		signal.Name(),
		trade.Symbol,
	}

	value.Append([]float64{
		retreatFraction,
		withdrawalRate,
	}, append(keys, signal.keys...)...)

	audit.Record.Write(map[string]any{
		"signal":          signal.Name(),
		"trade":           trade,
		"retreatFraction": retreatFraction,
		"withdrawalRate":  withdrawalRate,
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
