package cvd

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

/*
Trade registers executed-flow metric Conns on the grid. Next only reads retained observations.
*/
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

	tradeCount, tradeCountBranch := hold(NewTradeCount())
	buyCount, buyCountBranch := hold(NewBuyCount())
	sellCount, sellCountBranch := hold(NewSellCount())
	buyQty, buyQtyBranch := hold(NewBuyQty())
	sellQty, sellQtyBranch := hold(NewSellQty())
	grossQty, grossQtyBranch := hold(NewGrossQty())
	netQty, netQtyBranch := hold(NewNetQty())
	buyNotional, buyNotionalBranch := hold(NewBuyNotional())
	sellNotional, sellNotionalBranch := hold(NewSellNotional())
	grossNotional, grossNotionalBranch := hold(NewGrossNotional())
	netNotional, netNotionalBranch := hold(NewNetNotional())
	meanNotional, meanNotionalBranch := hold(NewMeanNotional())
	cvd, cvdBranch := hold(NewCVD())
	cnd, cndBranch := hold(NewCND())
	epoch, epochBranch := hold(NewEpoch())
	signedCount, signedCountBranch := hold(NewSignedCount())
	signedNet, signedNetBranch := hold(NewSignedNet())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewFlow(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			tradeCountBranch, buyCountBranch, sellCountBranch,
			buyQtyBranch, sellQtyBranch, grossQtyBranch, netQtyBranch,
			buyNotionalBranch, sellNotionalBranch, grossNotionalBranch, netNotionalBranch,
			meanNotionalBranch, cvdBranch, cndBranch, epochBranch,
			signedCountBranch, signedNetBranch,
		),
		NewTradeCount(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	register(ingress, interests)
	register(tradeCount, nil)
	register(buyCount, nil)
	register(sellCount, nil)
	register(buyQty, nil)
	register(sellQty, nil)
	register(grossQty, nil)
	register(netQty, nil)
	register(buyNotional, nil)
	register(sellNotional, nil)
	register(grossNotional, nil)
	register(netNotional, nil)
	register(meanNotional, nil)
	register(cvd, nil)
	register(cnd, nil)
	register(epoch, nil)
	register(signedCount, nil)
	register(signedNet, nil)

	trade := &Trade{grid: grid}
	trade.System = runtime.NewSystem(ctx, "cvd:trade", trade)
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
