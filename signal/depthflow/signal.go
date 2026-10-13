package depthflow

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type symbolState struct {
	lastMeasurement *data.Measurement
	prevBids        map[float64]float64
	prevAsks        map[float64]float64
	prevFullBid     bool
	prevFullAsk     bool
	prevBidWorst    float64
	prevAskWorst    float64
	prevAt          time.Time
	hasPrev         bool
}

type Signal struct {
	*runtime.System
	states map[string]*symbolState
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		states: make(map[string]*symbolState),
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)

	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	state, found := signal.states[prior.Label]

	if !found {
		state = &symbolState{
			prevBids: make(map[float64]float64),
			prevAsks: make(map[float64]float64),
		}
		signal.states[prior.Label] = state
	}

	// The book is the snapshot ingress stamped onto the trade at its sequence
	// number: every level of the depth window as the trade found it.
	snapshot, err := broker.ReadSnapshot(prior)

	if err != nil {
		signal.Error(errnie.Err(errnie.Internal, "[depthflow] stamped book unreadable", err))
		return nil
	}

	if snapshot == nil {
		if state.lastMeasurement != nil {
			return state.lastMeasurement
		}

		return nil
	}

	bids := ladder(snapshot.Bids)
	asks := ladder(snapshot.Asks)
	fullBid := snapshot.Full(snapshot.Bids)
	fullAsk := snapshot.Full(snapshot.Asks)

	touchBid := bids[0][0] * bids[0][1]

	obsBid := 0.0
	currentBids := make(map[float64]float64, len(bids))

	for _, b := range bids {
		notional := b[0] * b[1]
		obsBid += notional
		currentBids[b[0]] = notional
	}

	obsAsk := 0.0
	currentAsks := make(map[float64]float64, len(asks))

	for _, a := range asks {
		notional := a[0] * a[1]
		obsAsk += notional
		currentAsks[a[0]] = notional
	}

	total := obsBid + obsAsk
	bookImb := (obsBid - obsAsk) / total

	out := map[string]float64{
		"book_imbalance":     bookImb,
		"touch_notional:bid": touchBid,
	}

	if state.hasPrev {
		bidWorst := bids[len(bids)-1][0]
		askWorst := asks[len(asks)-1][0]

		addedBid, removedBid := levelFlow(
			state.prevBids, currentBids, state.prevFullBid, fullBid,
			state.prevBidWorst, bidWorst, func(price, edge float64) bool { return price >= edge },
		)
		addedAsk, removedAsk := levelFlow(
			state.prevAsks, currentAsks, state.prevFullAsk, fullAsk,
			state.prevAskWorst, askWorst, func(price, edge float64) bool { return price <= edge },
		)

		netBid := addedBid - removedBid
		out["net_displayed_flow:bid"] = netBid

		bidActivity := addedBid + removedBid
		askActivity := addedAsk + removedAsk
		bookActivity := bidActivity + askActivity

		if bookActivity > 0 {
			out["mutation_activity_imbalance"] = (bidActivity - askActivity) / bookActivity
		}
	}

	state.prevBids = currentBids
	state.prevAsks = currentAsks
	state.prevFullBid = fullBid
	state.prevFullAsk = fullAsk
	state.prevBidWorst = bids[len(bids)-1][0]
	state.prevAskWorst = asks[len(asks)-1][0]
	prevAt := state.prevAt
	hadPrev := state.hasPrev
	state.prevAt = prior.At
	state.hasPrev = true

	if state.lastMeasurement == nil {
		state.lastMeasurement = data.NewMeasurement(prior.Epoch, prior.Label, signal.Name(), prior.SeqIdx, prior.Tick)
	}

	res := state.lastMeasurement.Next(prior, out)
	state.lastMeasurement = res

	if hadPrev {
		res.From = prevAt
	}

	return res
}

/*
ladder converts one stamped side, best first, to (price, quantity) pairs.
*/
func ladder(levels []broker.Level) [][2]float64 {
	pairs := make([][2]float64, len(levels))

	for index, level := range levels {
		pairs[index] = [2]float64{level.Price.Float64(), level.Quantity.Float64()}
	}

	return pairs
}

/*
levelFlow returns notional added and removed between two windows of one book
side. A price only in the previous window is removed unless the current side
is full and the price lies beyond its edge (it left the window, not the book);
a price only in the current window is added unless the previous side was full
and the price lies beyond that edge. inside reports whether price lies within
the window ending at edge.
*/
func levelFlow(
	prev, current map[float64]float64,
	prevFull, currentFull bool,
	prevEdge, currentEdge float64,
	inside func(price, edge float64) bool,
) (added, removed float64) {
	for price, notional := range current {
		before, seen := prev[price]

		if !seen {
			if !prevFull || inside(price, prevEdge) {
				added += notional
			}

			continue
		}

		if notional > before {
			added += notional - before
		}

		if notional < before {
			removed += before - notional
		}
	}

	for price, before := range prev {
		if _, held := current[price]; held {
			continue
		}

		if !currentFull || inside(price, currentEdge) {
			removed += before
		}
	}

	return added, removed
}
