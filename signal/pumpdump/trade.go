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

type Trade struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTrade(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Trade {
	hold := func(extractor core.Primitive) (*transport.Conn[*geometry.Coordinate], core.Primitive) {
		retained := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(extractor, retained))
		return conn, nomagique.NewNumber(extractor, retained, transport.NewDiscard())
	}

	price, priceBranch := hold(NewTradePrice())
	qty, qtyBranch := hold(NewTradeQuantity())
	notional, notionalBranch := hold(NewTradeNotional())
	interval, intervalBranch := hold(NewTradeInterval())
	target, targetBranch := hold(NewBarTarget())
	barQty, barQtyBranch := hold(NewBarQuantity())
	barNotional, barNotionalBranch := hold(NewBarNotional())
	barCount, barCountBranch := hold(NewBarTradeCount())
	duration, durationBranch := hold(NewBarDuration())
	volumeRate, volumeRateBranch := hold(NewVolumeRate())
	notionalRate, notionalRateBranch := hold(NewNotionalRate())
	tradeRate, tradeRateBranch := hold(NewTradeRate())
	completed, completedBranch := hold(NewCompletedBars())
	rateBase, rateBaseBranch := hold(NewNotionalRateBaseline())
	rateRatio, rateRatioBranch := hold(NewNotionalRateRatio())
	rateDiv, rateDivBranch := hold(NewNotionalRateDivergence())
	rateZ, rateZBranch := hold(NewNotionalRateZScore())

	ingressStages := []core.Primitive{}

	if symbol != "" {
		ingressStages = append(ingressStages, store.NewOrigin(symbol))
	}

	ingressStages = append(ingressStages,
		NewTradeAssemble(),
		NewClock(),
		transport.NewFan(
			transport.NewIO[any](nil, nil),
			priceBranch, qtyBranch, notionalBranch, intervalBranch,
			targetBranch, barQtyBranch, barNotionalBranch, barCountBranch, durationBranch,
			volumeRateBranch, notionalRateBranch, tradeRateBranch, completedBranch,
			rateBaseBranch, rateRatioBranch, rateDivBranch, rateZBranch,
		),
		NewTradePrice(),
		store.NewRetained[float64](),
	)

	ingress := transport.NewConn[*geometry.Coordinate](nomagique.NewNumber(ingressStages...))
	shared.Register(grid, ingress, shared.Trade)
	shared.Register(grid, price, nil)
	shared.Register(grid, qty, nil)
	shared.Register(grid, notional, nil)
	shared.Register(grid, interval, nil)
	shared.Register(grid, target, nil)
	shared.Register(grid, barQty, nil)
	shared.Register(grid, barNotional, nil)
	shared.Register(grid, barCount, nil)
	shared.Register(grid, duration, nil)
	shared.Register(grid, volumeRate, nil)
	shared.Register(grid, notionalRate, nil)
	shared.Register(grid, tradeRate, nil)
	shared.Register(grid, completed, nil)
	shared.Register(grid, rateBase, nil)
	shared.Register(grid, rateRatio, nil)
	shared.Register(grid, rateDiv, nil)
	shared.Register(grid, rateZ, nil)

	trade := &Trade{grid: grid}
	trade.System = runtime.NewSystem(ctx, "pumpdump:trade", trade)
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
