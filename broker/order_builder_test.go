package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func TestBuildAddOrder(t *testing.T) {
	Convey("Given playbook actions", t, func() {
		action := logic.NewAction(
			logic.ActionMarket,
			trading.Buy,
			"BTC/USD",
			0,
			0,
			0,
			0.25,
			"5/0/0/0/0/0",
		)

		Convey("It should omit limit_price for market orders", func() {
			params, err := BuildAddOrder(action, OrderContext{Mark: 50_000}, 0.01, "abc", "token", nil)

			So(err, ShouldBeNil)
			So(params.OrderType, ShouldEqual, trading.Market)
			So(params.LimitPrice, ShouldEqual, 0)
			So(params.Triggers, ShouldBeNil)
		})

		Convey("It should map settle_position to the Kraken order type", func() {
			settle := logic.NewAction(
				logic.ActionSettlePosition,
				trading.Sell,
				"BTC/USD",
				0,
				0,
				0,
				1,
				"0",
			)

			params, err := BuildAddOrder(settle, OrderContext{Mark: 50_000}, 0.01, "abc", "token", nil)

			So(err, ShouldBeNil)
			So(params.OrderType, ShouldEqual, trading.SettlePosition)
			So(params.LimitPrice, ShouldEqual, 0)
		})

		Convey("It should require trigger price for stop-loss orders", func() {
			stop := logic.NewAction(
				logic.ActionStopLoss,
				trading.Sell,
				"BTC/USD",
				0,
				0,
				0,
				1,
				"0",
			)

			_, err := BuildAddOrder(stop, OrderContext{Mark: 50_000}, 0.01, "abc", "token", nil)

			So(err, ShouldNotBeNil)
		})

		Convey("It should attach triggers for stop-loss orders", func() {
			stop := logic.NewAction(
				logic.ActionStopLoss,
				trading.Sell,
				"BTC/USD",
				49_000,
				0,
				0,
				1,
				"0",
			)

			params, err := BuildAddOrder(stop, OrderContext{Mark: 50_000}, 0.01, "abc", "token", nil)

			So(err, ShouldBeNil)
			So(params.Triggers, ShouldNotBeNil)
			So(params.Triggers.Price, ShouldEqual, 49_000)
		})
	})
}

func TestPreTradeGateCheckEntry(t *testing.T) {
	Convey("Given configured quote limits", t, func() {
		viper.Set("trading.max_quote_age", 15*time.Second)
		viper.Set("trading.max_spread_bps", 120.0)
		viper.Set("trading.max_slippage_bps", 80.0)

		gate := &PreTradeGate{}
		action := logic.NewAction(
			logic.ActionMarket,
			trading.Buy,
			"BTC/USD",
			0,
			0,
			0,
			0.25,
			"5",
		)

		freshQuote := QuoteSnapshot{
			Symbol:    "BTC/USD",
			Mark:      50_000,
			Bid:       49_990,
			Ask:       50_010,
			UpdatedAt: time.Now(),
		}

		Convey("It should allow fresh tight quotes", func() {
			So(gate.CheckEntry(action, freshQuote), ShouldBeNil)
		})

		Convey("It should reject stale quotes", func() {
			stale := freshQuote
			stale.UpdatedAt = time.Now().Add(-30 * time.Second)

			So(gate.CheckEntry(action, stale), ShouldEqual, ErrQuoteStale)
		})

		Convey("It should bypass gates for exits", func() {
			exit := logic.NewAction(
				logic.ActionSettlePosition,
				trading.Sell,
				"BTC/USD",
				0,
				0,
				0,
				1,
				"0",
			)
			stale := freshQuote
			stale.UpdatedAt = time.Now().Add(-30 * time.Second)

			So(gate.CheckEntry(exit, stale), ShouldBeNil)
		})
	})
}
