package derivatives

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

type Trade struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTrade(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Trade {
	interests := [][]string{
		{"futures", "data", "symbol"},
		{"futures", "data", "side"},
		{"futures", "data", "price"},
		{"futures", "data", "qty"},
		{"futures", "data", "type"},
		{"futures", "data", "timestamp"},
	}

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	gross, grossBranch := hold(NewGrossTradeNotional())
	buy, buyBranch := hold(NewLiquidationBuy())
	sell, sellBranch := hold(NewLiquidationSell())
	liqGross, liqGrossBranch := hold(NewGrossLiquidation())
	net, netBranch := hold(NewNetLiquidation())
	share, shareBranch := hold(NewLiquidationShare())
	signed, signedBranch := hold(NewLiquidationSigned())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewTradeAssemble(),
		NewLiquidation(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			grossBranch, buyBranch, sellBranch, liqGrossBranch, netBranch,
			shareBranch, signedBranch,
		),
		NewGrossTradeNotional(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, interests)
	shared.Register(grid, gross, nil)
	shared.Register(grid, buy, nil)
	shared.Register(grid, sell, nil)
	shared.Register(grid, liqGross, nil)
	shared.Register(grid, net, nil)
	shared.Register(grid, share, nil)
	shared.Register(grid, signed, nil)

	trade := &Trade{grid: grid}
	trade.System = runtime.NewSystem(ctx, "derivatives:trade", trade)
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
