package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/observability"
)

func TestDeskRecordsOrderOperationalMetrics(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a desk action and execution lifecycle", t, func() {
		observability.ResetSharedForTest()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		defer pool.Close()

		desk := NewDesk(ctx, pool)
		defer func() { _ = desk.Close() }()

		now := time.Now().UTC()
		gate, gateOK := desk.riskGate.(*TickerPreTradeRiskGate)

		So(gateOK, ShouldBeTrue)
		seedSpreadHistory(gate, "BTC/USD", 100, 100.1, now)

		desk.onTicker(&market.TickerUpdate{
			Symbol:    "BTC/USD",
			Bid:       100,
			Ask:       100.1,
			Last:      100.1,
			Timestamp: time.Now().UTC().Add(-time.Millisecond),
		})

		action := &logic.Action{
			Type:     logic.ActionLimit,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Price:    100.1,
			Quantity: 0.1,
		}

		desk.onAction(action)

		So(action.ActionID, ShouldNotBeBlank)
		So(action.DecisionID, ShouldNotBeBlank)
		So(action.ClOrdID, ShouldNotBeBlank)

		desk.onExecution(user.Execution{
			ClOrdID:     action.ClOrdID,
			OrderID:     "order-1",
			Symbol:      "BTC/USD",
			ExecType:    "new",
			OrderStatus: "new",
		})
		desk.onExecution(user.Execution{
			ClOrdID:     action.ClOrdID,
			OrderID:     "order-1",
			ExecID:      "exec-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			LastPrice:   100.1,
			CumQty:      0.1,
		})

		snapshot := observability.Shared().Snapshot()

		Convey("It should record order IDs, latencies, and exposure", func() {
			So(snapshot.Orders.Submitted, ShouldEqual, 1)
			So(snapshot.Orders.Acknowledged, ShouldEqual, 1)
			So(snapshot.Orders.Filled, ShouldEqual, 1)
			So(len(snapshot.Orders.Correlations), ShouldEqual, 1)
			So(snapshot.Orders.Correlations[0].ExecutionID, ShouldEqual, "exec-1")
			So(len(snapshot.MarketData), ShouldEqual, 1)
			So(snapshot.Exposure.OpenPositions, ShouldEqual, 1)
		})
	})
}

func TestDeskRecordsStopOperationalMetrics(t *testing.T) {
	Convey("Given a stop triggered by a ticker", t, func() {
		observability.ResetSharedForTest()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		defer pool.Close()

		desk := NewDesk(ctx, pool)
		defer func() { _ = desk.Close() }()

		stopLoss, stopErr := NewStopLoss(
			"BTC/USD",
			0.5,
			100,
			0,
			desk.exitConfig.Load(),
		)

		So(stopErr, ShouldBeNil)
		desk.stops.Store("BTC/USD", stopLoss)

		desk.onTicker(&market.TickerUpdate{
			Symbol:    "BTC/USD",
			Bid:       stopLoss.StopPrice - 1,
			Ask:       stopLoss.StopPrice,
			Timestamp: time.Now().UTC(),
		})

		snapshot := observability.Shared().Snapshot()

		Convey("It should record stop trigger and exit-submit latency", func() {
			So(snapshot.Stops.Triggered, ShouldEqual, 1)
			So(snapshot.Stops.ExitSubmitted, ShouldEqual, 1)
		})
	})
}
