package hawkes

import (
	"context"
	"iter"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
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
		{"trade", "data", "symbol"},
		{"trade", "data", "side"},
		{"trade", "data", "timestamp"},
	}

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	count, countBranch := hold(NewEventCount())
	buyCount, buyCountBranch := hold(NewBuyCount())
	sellCount, sellCountBranch := hold(NewSellCount())
	buyFrac, buyFracBranch := hold(NewBuyFraction())
	sellFrac, sellFracBranch := hold(NewSellFraction())
	rate, rateBranch := hold(NewArrivalRate())
	buyRate, buyRateBranch := hold(NewBuyRate())
	sellRate, sellRateBranch := hold(NewSellRate())
	lambda, lambdaBranch := hold(NewConditionalIntensity())
	lambdaBuy, lambdaBuyBranch := hold(NewBuyIntensity())
	lambdaSell, lambdaSellBranch := hold(NewSellIntensity())
	radius, radiusBranch := hold(NewSpectralRadius())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		nmhawkes.NewProcess(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			countBranch, buyCountBranch, sellCountBranch,
			buyFracBranch, sellFracBranch,
			rateBranch, buyRateBranch, sellRateBranch,
			lambdaBranch, lambdaBuyBranch, lambdaSellBranch, radiusBranch,
		),
		NewEventCount(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, interests)
	shared.Register(grid, count, nil)
	shared.Register(grid, buyCount, nil)
	shared.Register(grid, sellCount, nil)
	shared.Register(grid, buyFrac, nil)
	shared.Register(grid, sellFrac, nil)
	shared.Register(grid, rate, nil)
	shared.Register(grid, buyRate, nil)
	shared.Register(grid, sellRate, nil)
	shared.Register(grid, lambda, nil)
	shared.Register(grid, lambdaBuy, nil)
	shared.Register(grid, lambdaSell, nil)
	shared.Register(grid, radius, nil)

	trade := &Trade{grid: grid}
	trade.System = runtime.NewSystem(ctx, "hawkes:trade", trade)
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
