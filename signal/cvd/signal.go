package cvd

import (
	"context"
	"math"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/volumeclock"
)

type window struct {
	buyNotional  float64
	sellNotional float64
}

type symbolState struct {
	clock        volumeclock.Clock
	window       window
	prevMidpoint float64
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
			"midpoint_log_return",
			"flow_aligned_midpoint_return",
			"gross_notional",
			"signed_net_fraction",
		},
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
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

	side := trade.Side

	if side != "buy" && side != "sell" {
		return value
	}

	price := trade.Price.Float64()
	qty := trade.Qty

	if price <= 0 || qty <= 0 {
		return value
	}

	state, found := signal.states[trade.Symbol]

	if !found {
		state = &symbolState{}
		signal.states[trade.Symbol] = state
	}

	tradeNotional := qty * price

	// The `books.Book` callback is synchronous; the book is present only if
	// the callback ran. Pending (no snapshot yet), stale (L3 socket down) and
	// invalid (venue checksum mismatch / absent level) symbols skip the
	// callback and the tick carries book_error/<symbol> instead.
	signal.books.Book(trade.Symbol, func(b *spotbook.Book) {
		bid := b.BestBid()
		ask := b.BestAsk()
		if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
			return
		}

		midpoint := (bid.Price.Float64() + ask.Price.Float64()) / 2.0
		tick := state.clock.Step(price, qty, float64(value.Timestamp), midpoint)

		if tick.Counted {
			switch side {
			case "buy":
				state.window.buyNotional += tradeNotional
			case "sell":
				state.window.sellNotional += tradeNotional
			}
		}

		var grossNotional, signedFraction float64

		if tick.Closed {
			flow := state.window
			state.window = window{}
			grossNotional = flow.buyNotional + flow.sellNotional
			netNotional := flow.buyNotional - flow.sellNotional
			if grossNotional > 0 {
				signedFraction = netNotional / grossNotional
			}
		}

		var midpointLogReturn, flowAligned float64

		if state.prevMidpoint > 0 {
			midpointLogReturn = math.Log(midpoint / state.prevMidpoint)
			flowAligned = midpointLogReturn

			if side == "sell" {
				flowAligned = -midpointLogReturn
			}
		}

		state.prevMidpoint = midpoint

		keys := []string{
			signal.Name(),
			trade.Symbol,
		}

		value.Append([]float64{
			midpointLogReturn,
			flowAligned,
			grossNotional,
			signedFraction,
		}, append(keys, signal.keys...)...)
	})

	return value
}
