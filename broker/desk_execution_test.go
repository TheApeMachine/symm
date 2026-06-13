package broker

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

func TestEconomicEntryPrice(t *testing.T) {
	Convey("Given a buy fill with fees", t, func() {
		entry := economicEntryPrice(user.Execution{
			Side:        string(trading.Buy),
			FeeUsdEquiv: 0.26,
		}, 1, 100)

		Convey("It should include the fee in the per-unit entry", func() {
			So(entry, ShouldAlmostEqual, 100.26, 1e-9)
		})
	})

	Convey("Given a sell fill with fees", t, func() {
		entry := economicEntryPrice(user.Execution{
			Side:        string(trading.Sell),
			FeeUsdEquiv: 0.26,
		}, 1, 100)

		Convey("It should subtract the fee from the per-unit exit price", func() {
			So(entry, ShouldAlmostEqual, 99.74, 1e-9)
		})
	})
}

func TestDeskEntryStopUsesQuoteSpread(t *testing.T) {
	Convey("Given a desk with a live quote at fill time", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

		const spreadBps = 50

		seedEntryStopQuote(desk, touchRegistry, "BTC/USD", 100, spreadBps)

		desk.actions.Store("entry-1", &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Quantity: 0.5,
		})
		desk.onExecution(user.Execution{
			ClOrdID:     "entry-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			LastPrice:   100,
			CumQty:      0.5,
		})

		Convey("It should arm the stop with spread-derived trail width", func() {
			raw, armed := desk.stops.Load("BTC/USD")

			So(armed, ShouldBeTrue)

			stopLoss, stopOK := raw.(*StopLoss)

			So(stopOK, ShouldBeTrue)
			So(stopLoss.Offset, ShouldBeGreaterThan, 0)
			So(stopLoss.StopPrice, ShouldBeLessThan, stopLoss.EntryPrice)
		})
	})
}
