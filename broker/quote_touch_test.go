package broker

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/internal/testconfig"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	symmmarket "github.com/theapemachine/symm/market"
)

func TestDeskRefreshesQuoteFromBook(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a desk with spread history and a fresh shared touch", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

		now := time.Now().UTC()
		gate, gateOK := desk.riskGate.(*TickerPreTradeRiskGate)

		So(gateOK, ShouldBeTrue)
		seedSpreadHistory(gate, "SOSO/USD", 1.05, 1.06, now)
		touchRegistry.SeedTouch(symmmarket.TouchSnapshot{
			Symbol:     "SOSO/USD",
			Bid:        1.05,
			Ask:        1.06,
			Last:       1.055,
			ObservedAt: now,
		})

		Convey("It should accept a new entry against the fresh touch", func() {
			desk.onAction(&logic.Action{
				Type:     logic.ActionMarket,
				Side:     trading.Buy,
				Symbol:   "SOSO/USD",
				Price:    1.055,
				Quantity: 10,
			})

			actionOpen := false
			desk.actions.Range(func(any, any) bool {
				actionOpen = true
				return false
			})

			So(actionOpen, ShouldBeTrue)
		})
	})
}

func TestDeskEvictsExpiredQuote(t *testing.T) {
	Convey("Given a desk without a fresh shared touch", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, _ := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

		desk.onAction(&logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "SOSO/USD",
			Price:    1.01,
			Quantity: 10,
		})

		Convey("It should reject with quote required", func() {
			actionOpen := false
			desk.actions.Range(func(any, any) bool {
				actionOpen = true
				return false
			})

			So(actionOpen, ShouldBeFalse)
		})
	})
}

func TestDeskRefreshesQuoteLastFromTrade(t *testing.T) {
	Convey("Given a desk with a fresh shared touch", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)

		defer func() { _ = desk.Close() }()

		now := time.Now().UTC()
		touchRegistry.SeedTouch(symmmarket.TouchSnapshot{
			Symbol:     "TAO/USD",
			Bid:        400,
			Ask:        401,
			Last:       400.5,
			ObservedAt: now,
		})
		desk.syncTouchQuote("TAO/USD")

		rawQuote, ok := desk.quotes.Load("TAO/USD")

		So(ok, ShouldBeTrue)

		quote := rawQuote.(QuoteSnapshot)

		Convey("It should persist the shared touch last price", func() {
			So(quote.Last, ShouldEqual, 400.5)
			So(time.Since(quote.ObservedAt), ShouldBeLessThan, time.Second)
		})
	})
}

func TestDeskRefreshesPositionMarkFromBook(t *testing.T) {
	Convey("Given an armed long position", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		desk, touchRegistry := newTestDesk(t, ctx, pool)
		subscriber := internal.NewBus(
			ctx,
			pool,
			nil,
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelUI, "desk-book-position-test"),
			},
		)

		defer func() { _ = desk.Close() }()

		desk.actions.Store("entry-1", &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   "BTC/USD",
			Quantity: 1,
		})
		desk.onExecution(user.Execution{
			ClOrdID:     "entry-1",
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			ExecType:    "trade",
			OrderStatus: "filled",
			AvgPrice:    100,
			CumQty:      1,
		})

		_, _ = subscriber.Receive(internal.ChannelUI)

		Convey("It should refresh unrealized P/L from the shared touch", func() {
			SeedDeskQuoteReadiness(desk, touchRegistry, "BTC/USD", 101, 101.2, 101)
			desk.onTicker(&krakenmarket.TickerUpdate{
				Symbol: "BTC/USD",
				Bid:    101,
				Ask:    101.2,
				Last:   101,
			})

			row, receiveErr := subscriber.Receive(internal.ChannelUI)

			So(receiveErr, ShouldBeNil)
			So(row.Type, ShouldEqual, "positions")

			frame, frameOK := row.Value.(PositionMonitorFrame)

			So(frameOK, ShouldBeTrue)
			So(frame.Positions[0].Mark, ShouldEqual, 101)
			So(frame.Positions[0].Unrealized, ShouldEqual, 1)
		})
	})
}
