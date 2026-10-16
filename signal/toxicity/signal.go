package toxicity

import (
	"context"
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
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
	states map[string]*symbolState
	keys   []string
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
		keys: []string{
			"retreat_fraction_bid",
			"net_withdrawal_rate_bid",
		},
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)
	return signal
}

func (signal *Signal) Step(value *data.Value) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	price := data.Attribute[float64](value.Layers[0], "data", 0, "price")
	qty := data.Attribute[float64](value.Layers[0], "data", 0, "qty")
	side := data.Attribute[string](value.Layers[0], "data", 0, "side")

	snapshot, err := broker.ReadSnapshot(value)
	if err != nil || snapshot == nil || len(snapshot.Bids) == 0 {
		return value
	}

	bestBid, _ := snapshot.Touch()
	bid, bidQty := bestBid.Price.Float64(), bestBid.Quantity.Float64()

	sideIndicator := 0.0
	if side == "buy" {
		sideIndicator = 1.0
	} else if side == "sell" {
		sideIndicator = -1.0
	}

	atNano := float64(value.Timestamp)

	state, exists := signal.states[value.Scope]
	if !exists {
		state = &symbolState{}
		signal.states[value.Scope] = state
	}

	if !state.hasPrev {
		state.observe(bid, bidQty, atNano)
		return data.AppendVector(value, []float64{0, 0}, signal.keys...)
	}

	state.pendingTradeQty += qty

	if sideIndicator < 0 && price == state.prevBid {
		state.pendingMatchedBid += qty
	}

	if atNano <= state.prevAtNano {
		return data.AppendVector(value, []float64{0, 0}, signal.keys...)
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

	return data.AppendVector(value, []float64{
		retreatFraction,
		withdrawalRate,
	}, signal.keys...)
}

func (state *symbolState) observe(bid, bidQty, atNano float64) {
	state.hasPrev = true
	state.prevBid = bid
	state.prevBidQty = bidQty
	state.prevAtNano = atNano
	state.pendingTradeQty = 0
	state.pendingMatchedBid = 0
}
