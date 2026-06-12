package market

import (
	"context"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

func TestStoryPendingIntentDoesNotOpenHolding(t *testing.T) {
	Convey("Given a story action submission path", t, func() {
		story := newAccountTestStory(t)
		action := &logic.Action{
			Type:            logic.ActionMarket,
			Side:            trading.Buy,
			Symbol:          "BTC/USD",
			Quantity:        0.25,
			EntryConfidence: 0.82,
		}

		err := story.submitAction(action)

		Convey("It should track pending intent without opening a position", func() {
			So(err, ShouldBeNil)
			So(story.holdings.IsHolding("BTC/USD"), ShouldBeFalse)
			So(story.hasPendingIntent(action), ShouldBeTrue)
		})

		Convey("A confirmed balance should open the holding", func() {
			story.applyBalance(user.Balances{
				Currency:  "USD",
				Inventory: map[string]float64{"BTC": 0.25},
			})

			position, ok := story.holdings.HeldPosition("BTC/USD")

			So(ok, ShouldBeTrue)
			So(position.Quantity, ShouldEqual, 0.25)
			So(position.EntryConfidence, ShouldEqual, 0.82)
			So(story.hasPendingIntent(action), ShouldBeFalse)
		})
	})
}

func TestStoryExitIntentWaitsForBalanceConfirmation(t *testing.T) {
	Convey("Given an open holding and a submitted exit", t, func() {
		story := newAccountTestStory(t)
		story.holdings.SetPosition("BTC/USD", 0.25, 0.82)
		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     trading.Sell,
			Symbol:   "BTC/USD",
			Quantity: 0.25,
		}

		err := story.submitAction(action)

		Convey("It should keep holding until balances confirm the exit", func() {
			So(err, ShouldBeNil)
			So(story.holdings.IsHolding("BTC/USD"), ShouldBeTrue)
			So(story.hasPendingIntent(action), ShouldBeTrue)
		})

		Convey("A zero balance should close the holding", func() {
			story.applyBalance(user.Balances{
				Currency: "USD",
				Asset: []user.Balance{{
					Asset:   "BTC",
					Balance: 0,
				}},
			})

			So(story.holdings.IsHolding("BTC/USD"), ShouldBeFalse)
			So(story.hasPendingIntent(action), ShouldBeFalse)
		})
	})
}

func TestStoryRejectedExecutionClearsPendingIntent(t *testing.T) {
	Convey("Given a pending buy intent", t, func() {
		story := newAccountTestStory(t)
		action := &logic.Action{
			Type:            logic.ActionMarket,
			Side:            trading.Buy,
			Symbol:          "BTC/USD",
			Quantity:        0.25,
			EntryConfidence: 0.82,
		}

		story.markPendingIntent(action)
		story.applyExecution(user.Execution{
			Symbol:      "BTC/USD",
			Side:        string(trading.Buy),
			OrderStatus: "rejected",
		})

		Convey("It should clear pending state without opening holdings", func() {
			So(story.hasPendingIntent(action), ShouldBeFalse)
			So(story.holdings.IsHolding("BTC/USD"), ShouldBeFalse)
		})
	})
}

func newAccountTestStory(t *testing.T) *Story {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	pool := qpool.NewQ[any](ctx, 1, 4, nil)

	t.Cleanup(func() {
		cancel()
		pool.Close()
	})

	return &Story{
		ctx:            ctx,
		cancel:         cancel,
		bus:            internal.NewBus(ctx, pool, []internal.Channel{internal.ChannelRaw}, nil),
		holdings:       logic.NewHoldings(),
		pendingIntents: &sync.Map{},
	}
}
