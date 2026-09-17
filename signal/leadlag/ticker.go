package leadlag

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
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
Ticker registers lead-lag metric Conns on the grid. Next only reads retained observations.
*/
type Ticker struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate], measured, reference string) *Ticker {
	symbolLast := [][]string{
		{"ticker", "data", "symbol"},
		{"ticker", "data", "last"},
	}
	symbolLastTime := [][]string{
		{"ticker", "data", "symbol"},
		{"ticker", "data", "last"},
		{"ticker", "data", "timestamp"},
	}

	register := func(conn *transport.Conn[*geometry.Coordinate], interests [][]string) {
		sequence.Read[core.Connectable[*geometry.Coordinate]](
			nomagique.NewNumber(
				sequence.NewValues(interests),
				core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
					conn, core.Identify,
				),
				grid,
			).Next(nil),
		)
	}

	lastHold := store.NewRetained[float64]()
	lastStages := []core.Primitive{nmcorrelation.NewTick(), nmcorrelation.NewLastPrice(), lastHold}

	if measured != "" {
		lastStages = []core.Primitive{
			nmcorrelation.NewTick(),
			nmcorrelation.NewMatch(measured),
			nmcorrelation.NewLastPrice(),
			lastHold,
		}
	}

	lastPrice := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(lastStages...))
	register(lastPrice, symbolLast)

	if measured != "" && reference != "" {
		hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
			retained := store.NewRetained[float64]()
			conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
			return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
		}

		contemporaneous, contemporaneousBranch := hold(NewContemporaneous())
		best, bestBranch := hold(NewBestLagCorrelation())
		index, indexBranch := hold(NewBestLagIndex())
		gain, gainBranch := hold(NewAbsoluteGain())
		fraction, fractionBranch := hold(NewLagFraction())
		search, searchBranch := hold(NewSearchCount())
		prominence, prominenceBranch := hold(NewProminence())
		curvature, curvatureBranch := hold(NewCurvature())

		ingress := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(
				nmcorrelation.NewTick(),
				NewPair(measured, reference),
				transport.NewFan(
					transport.NewIO[any](nil, nil),
					contemporaneousBranch, bestBranch, indexBranch, gainBranch,
					fractionBranch, searchBranch, prominenceBranch, curvatureBranch,
				),
				NewBestLagCorrelation(),
				store.NewRetained[float64](),
			),
		)
		register(ingress, symbolLastTime)
		register(contemporaneous, nil)
		register(best, nil)
		register(index, nil)
		register(gain, nil)
		register(fraction, nil)
		register(search, nil)
		register(prominence, nil)
		register(curvature, nil)
	}

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "leadlag:ticker", ticker)
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
