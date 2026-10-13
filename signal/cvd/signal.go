package cvd

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/volumeclock"
)

/*
window is the executed flow of the open volume bar, split by aggressor side.
*/
type window struct {
	buyNotional  float64
	sellNotional float64
}

/*
symbolState windows its flow totals on the same volume clock as pumpdump: a
bar's target is the median prior trade quantity, fixed when it opens. Totals
are reported once per closed bar, never accumulated since an epoch.
*/
type symbolState struct {
	lastMeasurement    *data.Measurement
	lastBarMeasurement *data.Measurement
	clock              volumeclock.Clock
	window             window
	lastAt             time.Time
	prevMidpoint       float64
}

type Signal struct {
	*runtime.System
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "cvd", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil {
		return nil
	}

	key := prior.Label + "@" + strconv.FormatInt(prior.Epoch, 10)
	state, found := signal.states[key]

	if !found {
		state = &symbolState{}
		signal.states[key] = state
	}

	var side string

	if entry := data.Pull(prior.Meta("side")); entry != nil {
		side = entry.Value
	}

	if side != "buy" && side != "sell" {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		if prior.Source == "spot:trade" {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[signal.cvd] no side",
				nil,
			))
		}

		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	qtyEntry := data.Pull(prior.Read("qty"))

	if priceEntry == nil || priceEntry.Metric == nil || qtyEntry == nil || qtyEntry.Metric == nil {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	price := priceEntry.Metric.Raw()
	qty := qtyEntry.Metric.Raw()

	if price <= 0 || qty <= 0 {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	tradeNotional := qty * price

	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[cvd] stamped book unreadable", err))
		return nil
	}

	var midpoint float64

	hasBook := snapshot != nil

	if hasBook {
		bid, ask := snapshot.Touch()
		midpoint = (bid.Price.Float64() + ask.Price.Float64()) / 2.0
	}

	out := map[string]float64{}
	tick := state.clock.Step(price, qty, float64(prior.At.UnixNano()), midpoint)

	if tick.Counted {
		if side == "buy" {
			state.window.buyNotional += tradeNotional
		}

		if side == "sell" {
			state.window.sellNotional += tradeNotional
		}
	}

	if tick.Closed {
		state.closeWindow(out)
	}

	if hasBook && state.prevMidpoint > 0 {
		midpointLogReturn := math.Log(midpoint / state.prevMidpoint)
		flowAligned := midpointLogReturn

		if side == "sell" {
			flowAligned = -midpointLogReturn
		}

		out["midpoint_log_return"] = midpointLogReturn
		out["flow_aligned_midpoint_return"] = flowAligned
	}

	if hasBook {
		state.prevMidpoint = midpoint
	}

	state.lastAt = prior.At

	var res *data.Measurement

	if tick.Closed {
		if state.lastBarMeasurement == nil {
			state.lastBarMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
		}

		res = state.lastBarMeasurement.Next(prior, out)
		state.lastBarMeasurement = res
		res.From = time.Unix(0, int64(tick.Bar.StartNanos)).UTC()
	}

	if !tick.Closed {
		if state.lastMeasurement == nil {
			state.lastMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.At, prior.From)
		}

		res = state.lastMeasurement.Next(prior, out)
		state.lastMeasurement = res
	}

	return res
}

/*
closeWindow writes the closed bar's flow totals: gross notional and signed net fraction,
then starts the next bar empty.
*/
func (state *symbolState) closeWindow(out map[string]float64) {
	flow := state.window
	state.window = window{}

	grossNotional := flow.buyNotional + flow.sellNotional
	netNotional := flow.buyNotional - flow.sellNotional

	out["gross_notional"] = grossNotional

	if grossNotional > 0 {
		out["signed_net_fraction"] = netNotional / grossNotional
	}
}
