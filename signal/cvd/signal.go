package cvd

import (
	"context"
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	side := data.Attribute[string](value.Layers[0], "data", 0, "side")
	if side != "buy" && side != "sell" {
		return value
	}

	price := data.Attribute[float64](value.Layers[0], "data", 0, "price")
	qty := data.Attribute[float64](value.Layers[0], "data", 0, "qty")
	if price <= 0 || qty <= 0 {
		return value
	}

	state, found := signal.states[value.Scope]
	if !found {
		state = &symbolState{}
		signal.states[value.Scope] = state
	}

	tradeNotional := qty * price

	snapshot, _ := broker.ReadSnapshot(value)
	var midpoint float64
	hasBook := snapshot != nil
	if hasBook {
		bid, ask := snapshot.Touch()
		midpoint = (bid.Price.Float64() + ask.Price.Float64()) / 2.0
	}

	tick := state.clock.Step(price, qty, float64(value.Timestamp), midpoint)
	if tick.Counted {
		if side == "buy" {
			state.window.buyNotional += tradeNotional
		} else if side == "sell" {
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

	return data.AppendVector(value, []float64{
		midpointLogReturn,
		flowAligned,
		grossNotional,
		signedFraction,
	}, signal.keys...)
}
