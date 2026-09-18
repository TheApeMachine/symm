package morphology

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

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	distance, distanceBranch := hold(NewDistance())
	ks, ksBranch := hold(NewKS())
	concBid, concBidBranch := hold(NewBidConcentration())
	concAsk, concAskBranch := hold(NewAskConcentration())
	entBid, entBidBranch := hold(NewBidEntropy())
	entAsk, entAskBranch := hold(NewAskEntropy())
	change, changeBranch := hold(NewChange())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewShape(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			distanceBranch, ksBranch, concBidBranch, concAskBranch,
			entBidBranch, entAskBranch, changeBranch,
		),
		NewDistance(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, interests)
	shared.Register(grid, distance, nil)
	shared.Register(grid, ks, nil)
	shared.Register(grid, concBid, nil)
	shared.Register(grid, concAsk, nil)
	shared.Register(grid, entBid, nil)
	shared.Register(grid, entAsk, nil)
	shared.Register(grid, change, nil)

	level3 := &Level3{grid: grid}
	level3.System = runtime.NewSystem(ctx, "morphology:level3", level3)
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
