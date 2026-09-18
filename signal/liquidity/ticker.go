package liquidity

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
Ticker registers liquidity metric Conns on the grid. Next only reads retained observations.
*/
type Ticker struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTicker(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Ticker {
	interests := [][]string{
		{"ticker", "data", "symbol"},
		{"ticker", "data", "bid"},
		{"ticker", "data", "ask"},
		{"ticker", "data", "bid_qty"},
		{"ticker", "data", "ask_qty"},
		{"ticker", "data", "timestamp"},
	}

	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], *store.Retained[float64], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, retained, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	bid, bidHold, bidBranch := hold(NewBid())
	ask, _, askBranch := hold(NewAsk())
	bidQty, _, bidQtyBranch := hold(NewBidQty())
	askQty, _, askQtyBranch := hold(NewAskQty())
	mid, _, midBranch := hold(NewMidpoint())
	spread, _, spreadBranch := hold(NewSpread())
	rel, _, relBranch := hold(NewRelativeSpread())
	bidNotional, _, bidNotionalBranch := hold(NewBidNotional())
	askNotional, _, askNotionalBranch := hold(NewAskNotional())
	twoSided, _, twoSidedBranch := hold(NewTwoSided())
	imbalance, _, imbalanceBranch := hold(NewImbalance())
	bidBase, _, bidBaseBranch := hold(NewBidBaseline())
	askBase, _, askBaseBranch := hold(NewAskBaseline())
	spreadBase, _, spreadBaseBranch := hold(NewSpreadBaseline())
	bidRatio, _, bidRatioBranch := hold(NewBidRatio())
	askRatio, _, askRatioBranch := hold(NewAskRatio())
	spreadRatio, _, spreadRatioBranch := hold(NewSpreadRatio())
	bidDiv, _, bidDivBranch := hold(NewBidDivergence())
	askDiv, _, askDivBranch := hold(NewAskDivergence())
	spreadDiv, _, spreadDivBranch := hold(NewSpreadDivergence())
	bidNoise, _, bidNoiseBranch := hold(NewBidNoise())
	askNoise, _, askNoiseBranch := hold(NewAskNoise())
	spreadNoise, _, spreadNoiseBranch := hold(NewSpreadNoise())
	bidZ, _, bidZBranch := hold(NewBidZ())
	askZ, _, askZBranch := hold(NewAskZ())
	spreadZ, _, spreadZBranch := hold(NewSpreadZ())
	bidVel, _, bidVelBranch := hold(NewBidVelocity())
	askVel, _, askVelBranch := hold(NewAskVelocity())
	spreadVel, _, spreadVelBranch := hold(NewSpreadVelocity())
	bidSNR, _, bidSNRBranch := hold(NewBidVelSNR())
	askSNR, _, askSNRBranch := hold(NewAskVelSNR())
	spreadSNR, _, spreadSNRBranch := hold(NewSpreadVelSNR())

	_ = bidHold

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewAssemble(),
		NewTouch(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			bidBranch, askBranch, bidQtyBranch, askQtyBranch,
			midBranch, spreadBranch, relBranch,
			bidNotionalBranch, askNotionalBranch, twoSidedBranch, imbalanceBranch,
			bidBaseBranch, askBaseBranch, spreadBaseBranch,
			bidRatioBranch, askRatioBranch, spreadRatioBranch,
			bidDivBranch, askDivBranch, spreadDivBranch,
			bidNoiseBranch, askNoiseBranch, spreadNoiseBranch,
			bidZBranch, askZBranch, spreadZBranch,
			bidVelBranch, askVelBranch, spreadVelBranch,
			bidSNRBranch, askSNRBranch, spreadSNRBranch,
		),
		NewBid(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, interests)
	shared.Register(grid, bid, nil)
	shared.Register(grid, ask, nil)
	shared.Register(grid, bidQty, nil)
	shared.Register(grid, askQty, nil)
	shared.Register(grid, mid, nil)
	shared.Register(grid, spread, nil)
	shared.Register(grid, rel, nil)
	shared.Register(grid, bidNotional, nil)
	shared.Register(grid, askNotional, nil)
	shared.Register(grid, twoSided, nil)
	shared.Register(grid, imbalance, nil)
	shared.Register(grid, bidBase, nil)
	shared.Register(grid, askBase, nil)
	shared.Register(grid, spreadBase, nil)
	shared.Register(grid, bidRatio, nil)
	shared.Register(grid, askRatio, nil)
	shared.Register(grid, spreadRatio, nil)
	shared.Register(grid, bidDiv, nil)
	shared.Register(grid, askDiv, nil)
	shared.Register(grid, spreadDiv, nil)
	shared.Register(grid, bidNoise, nil)
	shared.Register(grid, askNoise, nil)
	shared.Register(grid, spreadNoise, nil)
	shared.Register(grid, bidZ, nil)
	shared.Register(grid, askZ, nil)
	shared.Register(grid, spreadZ, nil)
	shared.Register(grid, bidVel, nil)
	shared.Register(grid, askVel, nil)
	shared.Register(grid, spreadVel, nil)
	shared.Register(grid, bidSNR, nil)
	shared.Register(grid, askSNR, nil)
	shared.Register(grid, spreadSNR, nil)

	ticker := &Ticker{grid: grid}
	ticker.System = runtime.NewSystem(ctx, "liquidity:ticker", ticker)
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
