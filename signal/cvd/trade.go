package cvd

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/signal/shared"
)

/*
Trade registers executed-flow metric Conns on the grid. Next only reads retained observations.
*/
type Trade struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTrade(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Trade {
	hold := func(extractor core.Primitive) (
		*transport.Conn[*geometry.Coordinate], core.Primitive,
	) {
		retained := store.NewRetained[float64]()
		
		conn := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(extractor, retained),
		)

		return conn, nomagique.NewNumber(
			extractor, retained, transport.NewDiscard(),
		)
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
			tradeCountBranch,
			buyCountBranch,
			sellCountBranch,
			buyQtyBranch,
			sellQtyBranch,
			grossQtyBranch,
			netQtyBranch,
			buyNotionalBranch,
			sellNotionalBranch,
			grossNotionalBranch,
			netNotionalBranch,
			meanNotionalBranch,
			cvdBranch,
			cndBranch,
			epochBranch,
			signedCountBranch,
			signedNetBranch,
		),
		NewTradeCount(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, shared.Trade)
	shared.Register(grid, tradeCount, nil)
	shared.Register(grid, buyCount, nil)
	shared.Register(grid, sellCount, nil)
	shared.Register(grid, buyQty, nil)
	shared.Register(grid, sellQty, nil)
	shared.Register(grid, grossQty, nil)
	shared.Register(grid, netQty, nil)
	shared.Register(grid, buyNotional, nil)
	shared.Register(grid, sellNotional, nil)
	shared.Register(grid, grossNotional, nil)
	shared.Register(grid, netNotional, nil)
	shared.Register(grid, meanNotional, nil)
	shared.Register(grid, cvd, nil)
	shared.Register(grid, cnd, nil)
	shared.Register(grid, epoch, nil)
	shared.Register(grid, signedCount, nil)
	shared.Register(grid, signedNet, nil)

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
