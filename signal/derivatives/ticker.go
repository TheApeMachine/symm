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

type Ticker struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Ticker {
	interests := [][]string{
		{"futures", "data", "symbol"},
		{"futures", "data", "last"},
		{"futures", "data", "index_price"},
		{"futures", "data", "mark_price"},
		{"futures", "data", "open_interest"},
		{"futures", "data", "timestamp"},
	}

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	last, lastBranch := hold(NewDerivativePrice())
	index, indexBranch := hold(NewReferencePrice())
	oi, oiBranch := hold(NewOpenInterest())
	basis, basisBranch := hold(NewBasisValue())
	logBasis, logBasisBranch := hold(NewLogBasis())
	basisBase, basisBaseBranch := hold(NewBasisBaseline())
	basisZ, basisZBranch := hold(NewBasisZScore())
	oiChange, oiChangeBranch := hold(NewOIChange())
	oiGrowth, oiGrowthBranch := hold(NewOIGrowth())
	gap, gapBranch := hold(NewReturnGap())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewBasis(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			lastBranch, indexBranch, oiBranch, basisBranch, logBasisBranch,
			basisBaseBranch, basisZBranch, oiChangeBranch, oiGrowthBranch, gapBranch,
		),
		NewDerivativePrice(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, interests)
	shared.Register(grid, last, nil)
	shared.Register(grid, index, nil)
	shared.Register(grid, oi, nil)
	shared.Register(grid, basis, nil)
	shared.Register(grid, logBasis, nil)
	shared.Register(grid, basisBase, nil)
	shared.Register(grid, basisZ, nil)
	shared.Register(grid, oiChange, nil)
	shared.Register(grid, oiGrowth, nil)
	shared.Register(grid, gap, nil)

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "derivatives:ticker", ticker)
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
