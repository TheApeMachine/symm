package depthflow

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

type Level3 struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewLevel3(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Level3 {
	interests := [][]string{
		{"level3", "data", "symbol"},
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
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	bidNotional, bidNotionalBranch := hold(NewBidNotional())
	askNotional, askNotionalBranch := hold(NewAskNotional())
	notional, notionalBranch := hold(NewBookNotional())
	imbalance, imbalanceBranch := hold(NewBookImbalance())
	addedBid, addedBidBranch := hold(NewAddedBid())
	addedAsk, addedAskBranch := hold(NewAddedAsk())
	removedBid, removedBidBranch := hold(NewRemovedBid())
	removedAsk, removedAskBranch := hold(NewRemovedAsk())
	turnover, turnoverBranch := hold(NewTurnover())
	imbalanceBase, imbalanceBaseBranch := hold(NewImbalanceBaseline())
	imbalanceZ, imbalanceZBranch := hold(NewImbalanceZScore())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewBook(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			bidNotionalBranch, askNotionalBranch, notionalBranch, imbalanceBranch,
			addedBidBranch, addedAskBranch, removedBidBranch, removedAskBranch,
			turnoverBranch, imbalanceBaseBranch, imbalanceZBranch,
		),
		NewBidNotional(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	register(ingress, interests)
	register(bidNotional, nil)
	register(askNotional, nil)
	register(notional, nil)
	register(imbalance, nil)
	register(addedBid, nil)
	register(addedAsk, nil)
	register(removedBid, nil)
	register(removedAsk, nil)
	register(turnover, nil)
	register(imbalanceBase, nil)
	register(imbalanceZ, nil)

	level3 := &Level3{grid: grid}
	level3.System = runtime.NewSystem(ctx, "depthflow:level3", level3)
	level3.Transition(runtime.READY)
	return level3
}

func (level3 *Level3) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		if level3.Status() != runtime.READY {
			errnie.Warn(level3.Name() + ": Next called before READY; dropping event")
			return
		}

		address := transport.NewAddress[*geometry.Coordinate]()
		query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			address, core.Read,
		)

		for out := range level3.grid.Next(query.Next(nil)) {
			if !yield(out) {
				return
			}
		}
	}
}
