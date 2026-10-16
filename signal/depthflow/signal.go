package depthflow

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type symbolState struct {
	prevBids     map[float64]float64
	prevAsks     map[float64]float64
	prevFullBid  bool
	prevFullAsk  bool
	prevBidWorst float64
	prevAskWorst float64
	hasPrev      bool
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
			"book_imbalance",
			"touch_notional:bid",
			"touch_notional:ask",
			"touch_imbalance",
			"net_displayed_flow:bid",
			"mutation_activity_imbalance",
		},
	}

	signal.System = runtime.NewSystem(ctx, "depthflow", signal)
	return signal
}

/*
Step processes the book of every distinct symbol traded in the tick's frame;
each symbol writes its own (reserved) layer.
*/
func (signal *Signal) Step(value *data.Value) *data.Value {
	trades, err := data.AttributeOk[[]kraken.TradeData](value.Layers[0], "data")

	if err != nil {
		errnie.Error(err)
		return value
	}

	for _, symbol := range broker.Symbols(trades) {
		signal.step(value, symbol)
	}

	return value
}

func (signal *Signal) step(value *data.Value, symbol string) *data.Value {
	if signal.Status() != runtime.READY || value == nil || len(value.Layers) == 0 {
		return value
	}

	snapshot, err := broker.ReadSnapshot(value, symbol)
	if err != nil || snapshot == nil || len(snapshot.Bids) == 0 || len(snapshot.Asks) == 0 {
		return value
	}

	state, found := signal.states[symbol]
	if !found {
		state = &symbolState{
			prevBids: make(map[float64]float64),
			prevAsks: make(map[float64]float64),
		}
		signal.states[symbol] = state
	}

	bids := ladder(snapshot.Bids)
	asks := ladder(snapshot.Asks)
	fullBid := snapshot.Full(snapshot.Bids)
	fullAsk := snapshot.Full(snapshot.Asks)

	touchBid := bids[0][0] * bids[0][1]
	touchAsk := asks[0][0] * asks[0][1]

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
	var bookImb float64
	if total > 0 {
		bookImb = (obsBid - obsAsk) / total
	}

	var touchImb float64
	if touchTotal := touchBid + touchAsk; touchTotal > 0 {
		touchImb = (touchBid - touchAsk) / touchTotal
	}

	var netBid, activityImb float64
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

		netBid = addedBid - removedBid
		bidActivity := addedBid + removedBid
		askActivity := addedAsk + removedAsk
		bookActivity := bidActivity + askActivity

		if bookActivity > 0 {
			activityImb = (bidActivity - askActivity) / bookActivity
		}
	}

	state.prevBids = currentBids
	state.prevAsks = currentAsks
	state.prevFullBid = fullBid
	state.prevFullAsk = fullAsk
	state.prevBidWorst = bids[len(bids)-1][0]
	state.prevAskWorst = asks[len(asks)-1][0]
	state.hasPrev = true

	return data.AppendSymbolVector(value, symbol, []float64{
		bookImb,
		touchBid,
		touchAsk,
		touchImb,
		netBid,
		activityImb,
	}, signal.keys...)
}

func ladder(levels []broker.Level) [][2]float64 {
	pairs := make([][2]float64, len(levels))
	for index, level := range levels {
		pairs[index] = [2]float64{level.Price.Float64(), level.Quantity.Float64()}
	}
	return pairs
}

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
