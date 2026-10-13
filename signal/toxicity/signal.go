package toxicity

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type symbolState struct {
	lastMeasurement   *data.Measurement
	hasPrev           bool
	prevBid           float64
	prevBidQty        float64
	prevAtNano        float64
	prevAt            time.Time
	pendingTradeQty   float64
	pendingMatchedBid float64
}

type Signal struct {
	*runtime.System
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "toxicity", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY {
		errnie.Warn(signal.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if prior == nil || prior.Label == "" {
		errnie.Warn("[" + signal.Name() + "] nil or empty prior")
		return nil
	}

	price := data.Pull(prior.Read("price")).Metric.Raw()
	qty := data.Pull(prior.Read("qty")).Metric.Raw()

	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[toxicity] stamped book unreadable", err))
		return nil
	}

	if snapshot == nil {
		state, exists := signal.states[prior.Label]

		if exists && state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	bestBid, _ := snapshot.Touch()
	bid, bidQty := bestBid.Price.Float64(), bestBid.Quantity.Float64()

	sideIndicator := 0.0
	side := prior.Meta("side")

	if side == "buy" {
		sideIndicator = 1.0
	}

	if side == "sell" {
		sideIndicator = -1.0
	}

	atNano := float64(prior.At.UnixNano())

	state, exists := signal.states[prior.Label]

	if !exists {
		state = &symbolState{}
		signal.states[prior.Label] = state
	}

	out := map[string]float64{}

	emit := func(from time.Time) *data.Measurement {
		var res *data.Measurement

		if state.lastMeasurement == nil {
			res = data.NewFromPrior(prior, signal.Name(), out)
		}

		if res == nil {
			res = state.lastMeasurement.Next(out)
		}

		res.At = prior.At
		state.lastMeasurement = res

		if !from.IsZero() {
			res.From = from
		}

		return res
	}

	if !state.hasPrev {
		state.observe(bid, bidQty, atNano, prior.At)
		return emit(time.Time{})
	}

	state.pendingTradeQty += qty

	if sideIndicator < 0 && price == state.prevBid {
		state.pendingMatchedBid += qty
	}

	if atNano <= state.prevAtNano {
		return emit(time.Time{})
	}

	dt := (atNano - state.prevAtNano) / 1e9
	rated := core.Resolvable(dt)

	fill := math.Min(state.pendingMatchedBid, state.prevBidQty)
	unfilled := state.prevBidQty - fill

	if bid == state.prevBid {
		withdrawn := math.Max(unfilled-bidQty, 0)
		out["retreat_fraction:bid"] = 0

		if rated {
			out["net_withdrawal_rate:bid"] = withdrawn / dt
		}
	}

	if bid < state.prevBid {
		out["retreat_fraction:bid"] = unfilled / state.prevBidQty
	}

	from := state.prevAt
	state.observe(bid, bidQty, atNano, prior.At)

	return emit(from)
}

func (state *symbolState) observe(
	bid, bidQty, atNano float64, at time.Time,
) {
	state.hasPrev = true
	state.prevBid = bid
	state.prevBidQty = bidQty
	state.prevAtNano = atNano
	state.prevAt = at
	state.pendingTradeQty = 0
	state.pendingMatchedBid = 0
}
