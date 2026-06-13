package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func TestDeskOnExecutionLifecycle(t *testing.T) {
	Convey("Given a desk with a resting entry action", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		desk, _ := newTestDesk(t, ctx, pool)

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

func TestKrakenOrderTypePaperExit(t *testing.T) {
	Convey("Given paper trading mode", t, func() {
		orderType, err := krakenOrderType(&logic.Action{
			Type: logic.ActionTakeProfit,
		}, false, "paper")

		So(err, ShouldBeNil)
		So(orderType, ShouldEqual, trading.Market)

		Convey("Live trading keeps the exchange-native exit type", func() {
			liveType, liveErr := krakenOrderType(&logic.Action{
				Type: logic.ActionTakeProfit,
			}, false, "live")

			So(liveErr, ShouldBeNil)
			So(liveType, ShouldEqual, trading.TakeProfit)
		})
	})
}

func TestDeskPublishesPositionMonitorFrame(t *testing.T) {
	Convey("Given a desk with a UI subscriber", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)
		subscriber := internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "desk-position-test"),
			},
		)

		defer func() { _ = desk.Close() }()

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Quantity: 0.5,
		}

		desk.actions.Store("cl-1", action)
		desk.onExecution(user.Execution{
			ClOrdID:     "cl-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			AvgPrice:    100,
			CumQty:      0.5,
		})

		armedRow, armedErr := subscriber.Receive(internal.ChannelUI)

		So(armedErr, ShouldBeNil)
		So(armedRow.Type, ShouldEqual, "positions")

		armedFrame, armedOK := armedRow.Value.(PositionMonitorFrame)

		So(armedOK, ShouldBeTrue)
		So(armedFrame.Positions[0].Mark, ShouldEqual, 100)

		now := time.Now().UTC()
		touchRegistry.SeedTouch(symmmarket.TouchSnapshot{
			Symbol:     "BTC/USD",
			Bid:        102.9,
			Ask:        103.1,
			Last:       103,
			ObservedAt: now,
		})
		desk.onTicker(&market.TickerUpdate{
			Symbol: "BTC/USD",
			Last:   103,
			Bid:    102.9,
			Ask:    103.1,
		})

		tickerRow, tickerErr := subscriber.Receive(internal.ChannelUI)

		So(tickerErr, ShouldBeNil)
		So(tickerRow.Type, ShouldEqual, "positions")

		tickerFrame, tickerOK := tickerRow.Value.(PositionMonitorFrame)

		So(tickerOK, ShouldBeTrue)
		So(tickerFrame.Positions[0].Mark, ShouldEqual, 102.9)
		So(tickerFrame.ExitBalance, ShouldAlmostEqual, 1.45, 1e-9)
	})
}

func TestDeskRetainsTriggeredStopUntilExitConfirmation(t *testing.T) {
	Convey("Given an armed stop on a live position", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

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

		raw, armed := desk.stops.Load("BTC/USD")
		So(armed, ShouldBeTrue)

		stopLoss, stopOK := raw.(*StopLoss)
		So(stopOK, ShouldBeTrue)

		SeedDeskQuoteReadiness(
			desk,
			touchRegistry,
			"BTC/USD",
			stopLoss.StopPrice-10,
			stopLoss.StopPrice-9.99,
			stopLoss.StopPrice-9.995,
		)

		desk.onTicker(&market.TickerUpdate{
			Symbol: "BTC/USD",
			Bid:    stopLoss.StopPrice - 10,
			Ask:    stopLoss.StopPrice - 9.99,
		})

		Convey("It should keep the stop until the exit fill confirms", func() {
			raw, retained := desk.stops.Load("BTC/USD")
			So(retained, ShouldBeTrue)

			retainedStop, retainedOK := raw.(*StopLoss)
			So(retainedOK, ShouldBeTrue)
			So(retainedStop.Quantity, ShouldEqual, 0.5)
			So(retainedStop.State, ShouldEqual, StopExitSubmitted)
		})

		Convey("A rejected exit should leave the stop repairable", func() {
			desk.actions.Store("exit-1", &logic.Action{
				Type:     logic.ActionMarket,
				Side:     trading.Sell,
				Symbol:   "BTC/USD",
				Quantity: 0.5,
			})
			desk.onExecution(user.Execution{
				ClOrdID:     "exit-1",
				Symbol:      "BTC/USD",
				Side:        string(trading.Sell),
				OrderStatus: "rejected",
			})

			raw, retained := desk.stops.Load("BTC/USD")
			So(retained, ShouldBeTrue)

			retainedStop, retainedOK := raw.(*StopLoss)
			So(retainedOK, ShouldBeTrue)
			So(retainedStop.State, ShouldEqual, StopNeedsRepair)
			So(retainedStop.Quantity, ShouldEqual, 0.5)
		})
	})
}

func TestDeskStopFiresOnBookWithoutTicker(t *testing.T) {
	Convey("Given an armed stop on an illiquid symbol with no ticker stream", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

		desk.actions.Store("entry-1", &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BLUE/USD",
			Quantity: 100,
		})
		desk.onExecution(user.Execution{
			ClOrdID:     "entry-1",
			Symbol:      "BLUE/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			LastPrice:   1.0,
			CumQty:      100,
		})

		raw, armed := desk.stops.Load("BLUE/USD")
		So(armed, ShouldBeTrue)

		stopLoss := raw.(*StopLoss)
		breach := stopLoss.StopPrice * 0.9

		Convey("A book update below the stop should submit the exit", func() {
			SeedDeskQuoteReadiness(
				desk,
				touchRegistry,
				"BLUE/USD",
				breach,
				breach*1.001,
				breach,
			)
			desk.onBook(&market.BookUpdate{
				Symbol: "BLUE/USD",
				Type:   "snapshot",
				Bids:   []market.BookLevel{{Price: breach, Qty: 500}},
				Asks:   []market.BookLevel{{Price: breach * 1.001, Qty: 500}},
			})

			raw, retained := desk.stops.Load("BLUE/USD")
			So(retained, ShouldBeTrue)

			retainedStop := raw.(*StopLoss)
			So(retainedStop.State, ShouldEqual, StopExitSubmitted)
		})
	})
}

func TestDeskExecutionFillDeltas(t *testing.T) {
	Convey("Given cumulative entry execution updates", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, _ := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

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
			OrderStatus: "partial",
			LastQty:     0.2,
			LastPrice:   100,
			CumQty:      0.2,
		})
		desk.onExecution(user.Execution{
			ClOrdID:     "entry-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			LastQty:     0.3,
			LastPrice:   110,
			CumQty:      0.5,
		})

		Convey("It should arm one stop with cumulative position quantity", func() {
			raw, armed := desk.stops.Load("BTC/USD")
			So(armed, ShouldBeTrue)

			stopLoss, stopOK := raw.(*StopLoss)
			So(stopOK, ShouldBeTrue)
			So(stopLoss.Quantity, ShouldAlmostEqual, 0.5, 1e-12)
			So(stopLoss.EntryPrice, ShouldAlmostEqual, 106, 1e-12)

			_, actionOpen := desk.actions.Load("entry-1")
			So(actionOpen, ShouldBeFalse)
		})
	})

	Convey("Given cumulative exit execution updates", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, _ := newTestDesk(t, ctx, pool)

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
		desk.actions.Store("exit-1", &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Sell,
			Symbol:   "BTC/USD",
			Quantity: 0.5,
		})

		desk.onExecution(user.Execution{
			ClOrdID:     "exit-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Sell),
			ExecType:    "trade",
			OrderStatus: "partial",
			LastQty:     0.2,
			LastPrice:   99,
			CumQty:      0.2,
		})

		raw, retained := desk.stops.Load("BTC/USD")
		So(retained, ShouldBeTrue)

		retainedStop, retainedOK := raw.(*StopLoss)
		So(retainedOK, ShouldBeTrue)
		So(retainedStop.Quantity, ShouldAlmostEqual, 0.3, 1e-12)

		desk.onExecution(user.Execution{
			ClOrdID:     "exit-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Sell),
			ExecType:    "trade",
			OrderStatus: "filled",
			LastQty:     0.3,
			LastPrice:   99,
			CumQty:      0.5,
		})

		Convey("It should close the stop only after cumulative fill completes", func() {
			_, retained := desk.stops.Load("BTC/USD")
			So(retained, ShouldBeFalse)

			_, actionOpen := desk.actions.Load("exit-1")
			So(actionOpen, ShouldBeFalse)
		})
	})
}
