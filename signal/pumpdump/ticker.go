package pumpdump

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

type Ticker struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Ticker {
	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	bid, bidBranch := hold(NewBid())
	ask, askBranch := hold(NewAsk())
	mid, midBranch := hold(NewMidpoint())
	spread, spreadBranch := hold(NewSpread())
	rel, relBranch := hold(NewRelativeSpread())
	base, baseBranch := hold(NewSpreadBaseline())
	ratio, ratioBranch := hold(NewSpreadRatio())
	div, divBranch := hold(NewSpreadDivergence())
	zscore, zBranch := hold(NewSpreadZScore())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewTickerAssemble(),
		NewTouch(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			bidBranch, askBranch, midBranch, spreadBranch, relBranch,
			baseBranch, ratioBranch, divBranch, zBranch,
		),
		NewBid(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, shared.Ticker)
	shared.Register(grid, bid, nil)
	shared.Register(grid, ask, nil)
	shared.Register(grid, mid, nil)
	shared.Register(grid, spread, nil)
	shared.Register(grid, rel, nil)
	shared.Register(grid, base, nil)
	shared.Register(grid, ratio, nil)
	shared.Register(grid, div, nil)
	shared.Register(grid, zscore, nil)

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "pumpdump:ticker", ticker)
	ticker.Transition(runtime.READY)
	return ticker
}

func (ticker *Ticker) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		if ticker.Status() != runtime.READY {
			errnie.Warn(ticker.Name() + ": Next called before READY; dropping event")
			return
		}

		address := transport.NewAddress[*geometry.Coordinate]()
		query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
			address, core.Read,
		)

		for out := range ticker.grid.Next(query.Next(nil)) {
			if !yield(out) {
				return
			}
		}
	}
}
