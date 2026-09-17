package derivatives

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
	register(ingress, interests)
	register(last, nil)
	register(index, nil)
	register(oi, nil)
	register(basis, nil)
	register(logBasis, nil)
	register(basisBase, nil)
	register(basisZ, nil)
	register(oiChange, nil)
	register(oiGrowth, nil)
	register(gap, nil)

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
