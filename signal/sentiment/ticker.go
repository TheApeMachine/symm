package sentiment

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

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate], members ...string) *Ticker {
	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	last, lastBranch := hold(NewLast())
	symbol := ""

	if len(members) == 1 {
		symbol = members[0]
	}

	branches := []core.Primitive{transport.NewIO[any](nil, nil), lastBranch}
	conns := []*transport.Conn[*geometry.Coordinate]{last}

	if len(members) > 1 {
		valid, validBranch := hold(NewValidCount())
		advance, advanceBranch := hold(NewAdvanceCount())
		decline, declineBranch := hold(NewDeclineCount())
		unchanged, unchangedBranch := hold(NewUnchangedCount())
		breadth, breadthBranch := hold(NewBreadth())
		median, medianBranch := hold(NewMedianReturn())
		medianAbs, medianAbsBranch := hold(NewMedianAbsolute())
		mad, madBranch := hold(NewReturnMAD())
		largest, largestBranch := hold(NewLargestAbsolute())
		signed, signedBranch := hold(NewSignedFraction())
		signedBase, signedBaseBranch := hold(NewSignedBaseline())
		signedZ, signedZBranch := hold(NewSignedZScore())
		branches = append(branches,
			validBranch, advanceBranch, declineBranch, unchangedBranch,
			breadthBranch, medianBranch, medianAbsBranch, madBranch,
			largestBranch, signedBranch, signedBaseBranch, signedZBranch,
		)
		conns = append(conns,
			valid, advance, decline, unchanged, breadth, median, medianAbs,
			mad, largest, signed, signedBase, signedZ,
		)
	}

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewCohort(members...),
		transport.NewFan(branches...),
		NewLast(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, shared.Ticker)

	for _, conn := range conns {
		shared.Register(grid, conn, nil)
	}

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "sentiment:ticker", ticker)
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
