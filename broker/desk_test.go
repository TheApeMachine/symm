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

func TestDeskOnExecutionLifecycle(t *testing.T) {
	Convey("Given a desk with a resting entry action", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		desk := NewDesk(ctx, pool)

		defer func() { _ = desk.Close() }()

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Quantity: 0.5,
		}

		desk.actions.Store("cl-1", action)

		Convey("A pre-fill status row must not consume the action", func() {
			desk.onExecution(user.Execution{
				ClOrdID:     "cl-1",
				Symbol:      "BTC/USD",
				ExecType:    "new",
				OrderStatus: "new",
			})

			_, stillThere := desk.actions.Load("cl-1")
			So(stillThere, ShouldBeTrue)

			_, armed := desk.stops.Load("BTC/USD")
			So(armed, ShouldBeFalse)

			Convey("The eventual fill arms the stop and retires the action", func() {
				desk.onExecution(user.Execution{
					ClOrdID:     "cl-1",
					Symbol:      "BTC/USD",
					Side:        string(trading.Buy),
					ExecType:    "trade",
					OrderStatus: "filled",
					AvgPrice:    100,
					CumQty:      0.5,
				})

				_, stillThere := desk.actions.Load("cl-1")
				So(stillThere, ShouldBeFalse)

				raw, armed := desk.stops.Load("BTC/USD")
				So(armed, ShouldBeTrue)

				stop, isStop := raw.(*StopLoss)
				So(isStop, ShouldBeTrue)
				So(stop.Quantity, ShouldEqual, 0.5)
				So(stop.EntryPrice, ShouldEqual, 100)
			})
		})
	})
}
