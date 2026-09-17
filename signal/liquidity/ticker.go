package liquidity

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
	register(ingress, interests)
	register(bid, nil)
	register(ask, nil)
	register(bidQty, nil)
	register(askQty, nil)
	register(mid, nil)
	register(spread, nil)
	register(rel, nil)
	register(bidNotional, nil)
	register(askNotional, nil)
	register(twoSided, nil)
	register(imbalance, nil)
	register(bidBase, nil)
	register(askBase, nil)
	register(spreadBase, nil)
	register(bidRatio, nil)
	register(askRatio, nil)
	register(spreadRatio, nil)
	register(bidDiv, nil)
	register(askDiv, nil)
	register(spreadDiv, nil)
	register(bidNoise, nil)
	register(askNoise, nil)
	register(spreadNoise, nil)
	register(bidZ, nil)
	register(askZ, nil)
	register(spreadZ, nil)
	register(bidVel, nil)
	register(askVel, nil)
	register(spreadVel, nil)
	register(bidSNR, nil)
	register(askSNR, nil)
	register(spreadSNR, nil)

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
