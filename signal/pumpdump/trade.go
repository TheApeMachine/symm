package pumpdump

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

type Trade struct {
	*runtime.System
	grid *store.Grid[*geometry.Coordinate]
}

func NewTrade(ctx context.Context, grid *store.Grid[*geometry.Coordinate], symbol string) *Trade {
	interests := [][]string{
		{"trade", "data", "symbol"},
		{"trade", "data", "price"},
		{"trade", "data", "qty"},
		{"trade", "data", "timestamp"},
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
	register(ingress, interests)
	register(price, nil)
	register(qty, nil)
	register(notional, nil)
	register(interval, nil)
	register(target, nil)
	register(barQty, nil)
	register(barNotional, nil)
	register(barCount, nil)
	register(duration, nil)
	register(volumeRate, nil)
	register(notionalRate, nil)
	register(tradeRate, nil)
	register(completed, nil)
	register(rateBase, nil)
	register(rateRatio, nil)
	register(rateDiv, nil)
	register(rateZ, nil)

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
