package cvd

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
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
	states map[string]*symbolState
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
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
	trades, err := data.AttributeOk[[]kraken.TradeData](value.Layers[0], "data")

	if err != nil {
		errnie.Error(err)
		return value
	}

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

	snapshot, err := broker.ReadSnapshot(value, trade.Symbol)

	if err != nil {
		// The tick records this book as not valid (book_error layer) and
		// its transition was raised once where it happened: skip the symbol.
		if errors.Is(err, broker.ErrBookNotValid) {
			return value
		}

		// errnie.Error, not signal.Error: a per-tick condition must not
		// accumulate into the node's joined error forever.
		errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("[cvd] snapshot for %s unavailable", trade.Symbol),
			err,
		))

		return value
	}

	var midpoint float64

	hasBook := snapshot != nil

	if hasBook {
		bid, ask := snapshot.Touch()
		midpoint = (bid.Price.Float64() + ask.Price.Float64()) / 2.0
	}

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

	if hasBook && state.prevMidpoint > 0 {
		midpointLogReturn = math.Log(midpoint / state.prevMidpoint)
		flowAligned = midpointLogReturn

		if side == "sell" {
			flowAligned = -midpointLogReturn
		}
	}

	if hasBook {
		state.prevMidpoint = midpoint
	}

	return data.AppendSymbolVector(value, trade.Symbol, []float64{
		midpointLogReturn,
		flowAligned,
		grossNotional,
		signedFraction,
	}, signal.keys...)
}
