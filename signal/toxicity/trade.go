package toxicity

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

type Trade struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTrade(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Trade {
	interests := [][]string{
		{"trade", "data", "symbol"},
		{"trade", "data", "side"},
		{"trade", "data", "price"},
		{"trade", "data", "qty"},
		{"trade", "data", "timestamp"},
		{"level3", "data", "bid"},
		{"level3", "data", "ask"},
		{"level3", "data", "bid_qty"},
		{"level3", "data", "ask_qty"},
		{"level3", "data", "timestamp"},
	}

	register := func(conn *transport.Conn[*geometry.Coordinate], wanted [][]string) {
		sequence.Read[core.Connectable[*geometry.Coordinate]](
			nomagique.NewNumber(
				sequence.NewValues(wanted),
				core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				),
				grid,
			).Next(nil),
		)
	}

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewKeyed[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	qty, qtyBranch := hold(NewTradeQty())
	bracket, bracketBranch := hold(NewBracketQty())
	matchedBid, matchedBidBranch := hold(NewMatchedBidQty())
	matchedAsk, matchedAskBranch := hold(NewMatchedAskQty())
	fillBid, fillBidBranch := hold(NewFillBidQty())
	fillAsk, fillAskBranch := hold(NewFillAskQty())
	fillBidF, fillBidFBranch := hold(NewFillBidFraction())
	fillAskF, fillAskFBranch := hold(NewFillAskFraction())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewTradeAssemble(),
		NewMatch(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			qtyBranch, bracketBranch,
			matchedBidBranch, matchedAskBranch,
			fillBidBranch, fillAskBranch,
			fillBidFBranch, fillAskFBranch,
		),
		NewTradeQty(),
		store.NewKeyed[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	register(ingress, interests)
	register(qty, nil)
	register(bracket, nil)
	register(matchedBid, nil)
	register(matchedAsk, nil)
	register(fillBid, nil)
	register(fillAsk, nil)
	register(fillBidF, nil)
	register(fillAskF, nil)

	trade := &Trade{grid: grid}
	trade.System = runtime.NewSystem(ctx, "toxicity:trade", trade)
	trade.Transition(runtime.READY)
	return trade
}

func (trade *Trade) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		if trade.Status() != runtime.READY {
			errnie.Warn(trade.Name() + ": Next called before READY; dropping event")
			return
		}

		address := transport.NewAddress[*geometry.Coordinate]()
		query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			address, core.Read,
		)

		for out := range trade.grid.Next(query.Next(nil)) {
			if !yield(out) {
				return
			}
		}
	}
}
