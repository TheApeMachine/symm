package toxicity

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
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

	price := trade.Price.Float64()
	qty := trade.Qty
	side := trade.Side

	snapshot, err := broker.ReadSnapshot(value, trade.Symbol)
	if err != nil || snapshot == nil || len(snapshot.Bids) == 0 {
		return value
	}

	bestBid, _ := snapshot.Touch()
	bid, bidQty := bestBid.Price.Float64(), bestBid.Quantity.Float64()

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

	if !state.hasPrev {
		state.observe(bid, bidQty, atNano)
		return data.AppendSymbolVector(value, trade.Symbol, []float64{0, 0}, signal.keys...)
	}

	state.pendingTradeQty += qty

	if sideIndicator < 0 && price == state.prevBid {
		state.pendingMatchedBid += qty
	}

	if atNano <= state.prevAtNano {
		return data.AppendSymbolVector(value, trade.Symbol, []float64{0, 0}, signal.keys...)
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

	return data.AppendSymbolVector(value, trade.Symbol, []float64{
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
