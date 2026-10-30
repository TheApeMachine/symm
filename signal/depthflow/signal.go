package depthflow

import (
	"context"
	"sort"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	parent "github.com/theapemachine/symm/signal"
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
	books  *broker.Book
	states *sync.Map
	keys   []string
}

func NewSignal(ctx context.Context, books *broker.Book) *Signal {
	signal := &Signal{
		books:  books,
		states: &sync.Map{},
		keys: []string{
			"bid_ladder_build",
			"bid_ladder_release",
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
	var trades []kraken.TradeData
	value.Lookup(&trades, "spot:trade")

	symbols := parent.DistinctSymbols(trades)

	if err := parent.RunSymbols(signal.Context(), symbols, func(symbol string) {
		signal.step(value, symbol)
	}); err != nil {
		signal.Error(err)
	}

	return value
}

func (signal *Signal) step(value *data.Value, symbol string) *data.Value {
	if signal.Status() != runtime.READY || !parent.HasFrame(value) {
		return value
	}

	untypedState, _ := signal.states.LoadOrStore(symbol, &symbolState{
		prevBids: make(map[float64]float64),
		prevAsks: make(map[float64]float64),
	})

	state := untypedState.(*symbolState)

	// The `books.Book` callback is synchronous; the book is present only if
	// the callback ran. Pending (no snapshot yet), stale (L3 socket down) and
	// invalid (venue checksum mismatch / absent level) symbols skip the
	// callback and the tick carries book_error/<symbol> instead.
	signal.books.Book(symbol, func(b *spotbook.Book) {
		bids := ladder(b.Bids, true)
		asks := ladder(b.Asks, false)

		if len(bids) == 0 || len(asks) == 0 {
			return
		}

		fullBid := len(b.Bids.Levels) >= b.MaxDepth
		fullAsk := len(b.Asks.Levels) >= b.MaxDepth

		obsBid := 0.0
		currentBids := make(map[float64]float64, len(bids))
		for _, bid := range bids {
			notional := bid[0] * bid[1]
			obsBid += notional
			currentBids[bid[0]] = notional
		}

		obsAsk := 0.0
		currentAsks := make(map[float64]float64, len(asks))
		for _, ask := range asks {
			notional := ask[0] * ask[1]
			obsAsk += notional
			currentAsks[ask[0]] = notional
		}

		var netBid float64
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
			_ = addedAsk
			_ = removedAsk
		}

		state.prevBids = currentBids
		state.prevAsks = currentAsks
		state.prevFullBid = fullBid
		state.prevFullAsk = fullAsk
		state.prevBidWorst = bids[len(bids)-1][0]
		state.prevAskWorst = asks[len(asks)-1][0]
		state.hasPrev = true

		keys := []string{
			signal.Name(),
			symbol,
		}

		build := netBid

		if build < 0 {
			build = 0
		}

		release := -netBid

		if release < 0 {
			release = 0
		}

		value.Append([]float64{
			build,
			release,
		}, append(keys, signal.keys...)...)

		audit.Record.Write(map[string]any{
			"signal":  signal.Name(),
			"symbol":  symbol,
			"build":   build,
			"release": release,
		})
	})

	return value
}

/*
ladder returns a side's levels as price/quantity pairs, best first: bids
descending, asks ascending.
*/
func ladder(side *spotbook.Side, descending bool) [][2]float64 {
	if side == nil {
		return nil
	}

	pairs := make([][2]float64, 0, len(side.Levels))

	for _, level := range side.Levels {
		if level == nil || level.Price == nil || level.Quantity == nil {
			continue
		}
		pairs = append(pairs, [2]float64{level.Price.Float64(), level.Quantity.Float64()})
	}

	sort.Slice(pairs, func(left, right int) bool {
		if descending {
			return pairs[left][0] > pairs[right][0]
		}
		return pairs[left][0] < pairs[right][0]
	})

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
