package user

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

func TestPublishExecutionDerived(t *testing.T) {
	Convey("Given a raw broadcast group", t, func() {
		pool := qpool.NewQ(t.Context(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 0)
		sub := raw.Subscribe("test:derived", 4)

		Convey("Only trade rows produce derived envelopes", func() {
			PublishExecutionsRaw(raw, "update", []Execution{
				{Symbol: "BTC/EUR", ExecType: "new", Side: "buy"},
				{
					Symbol: "BTC/EUR", ExecType: "trade", Side: "buy",
					LastQty: 1, LastPrice: 100, OrderStatus: "filled",
					Fees: []ExecutionFee{{Asset: "EUR", Qty: 0.4}},
				},
			})

			var derived map[string]any

			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)

				if frame["qty"] != nil {
					derived = frame
				}
			default:
			}

			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)

				if frame["qty"] != nil {
					derived = frame
				}
			default:
			}

			So(derived, ShouldNotBeNil)
			So(derived["qty"], ShouldEqual, 1.0)
			So(derived["fee"], ShouldEqual, 0.4)
		})
	})
}

func TestHoldingsFromBalances(t *testing.T) {
	Convey("holdingsFromBalances maps held non-quote assets to trading pairs", t, func() {
		holdings := holdingsFromBalances([]Balance{
			{Asset: "EUR", Balance: 200},  // quote currency is cash, not a position
			{Asset: "ETC", Balance: 5.5},  // a real holding
			{Asset: "XLM", Balance: 0},    // zero balance is not held
			{Asset: "btc", Balance: 0.01}, // lower-case asset still maps
		}, "EUR")

		So(len(holdings), ShouldEqual, 2)
		So(holdings[0]["symbol"], ShouldEqual, "ETC/EUR")
		So(holdings[0]["qty"], ShouldEqual, 5.5)
		So(holdings[1]["symbol"], ShouldEqual, "BTC/EUR")
	})
}

func TestPublishHoldingsDerived(t *testing.T) {
	Convey("Given a raw broadcast group and an EUR quote currency", t, func() {
		viper.Set("market.quote_currency", "EUR")
		defer viper.Set("market.quote_currency", "")

		pool := qpool.NewQ(t.Context(), 1, 4, nil)
		raw := pool.CreateBroadcastGroup("raw", 0)
		sub := raw.Subscribe("test:holdings", 4)

		PublishHoldingsDerived(raw, []Balance{
			{Asset: "EUR", Balance: 200},
			{Asset: "ETC", Balance: 5},
		})

		Convey("It publishes one holdings frame carrying only the non-quote pair", func() {
			select {
			case msg := <-sub.Incoming:
				frame, _ := msg.Value.(map[string]any)
				So(frame["channel"], ShouldEqual, "holdings")

				holdings, ok := frame["holdings"].([]map[string]any)
				So(ok, ShouldBeTrue)
				So(len(holdings), ShouldEqual, 1)
				So(holdings[0]["symbol"], ShouldEqual, "ETC/EUR")
				So(holdings[0]["qty"], ShouldEqual, 5.0)
			case <-time.After(2 * time.Second):
				t.Fatal("no holdings frame published")
			}
		})
	})
}
