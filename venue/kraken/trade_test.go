package kraken

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestNewTrade(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTrade is called", func() {
			trade := NewTrade()
			req := trade.Request()

			Convey("defaults target the public trade channel", func() {
				So(trade.Channel, ShouldEqual, "trade")
				So(trade.Type, ShouldEqual, "update")
				So(req["method"], ShouldEqual, "subscribe")
				So(req["params"].(map[string]any)["channel"], ShouldEqual, "trade")
			})
		})
	})
}

func TestUnmarshalJSONTrade(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"channel":"trade",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"side":"buy",
				"price":"50000.1",
				"qty":0.25,
				"ord_type":"market",
				"trade_id":42,
				"timestamp":"2024-01-02T12:00:00Z"
			}]
		}`)
		trade := NewTrade()
		called := 0
		trade.OnChange(func(_ market.MarketType) { called++ })

		Convey("When a trade websocket frame arrives", func() {
			So(trade.UnmarshalJSON(raw), ShouldBeNil)

			Convey("tape fields decode and OnChange fires", func() {
				So(called, ShouldEqual, 1)
				So(trade.Data, ShouldHaveLength, 1)
				So(trade.Data[0].Symbol, ShouldEqual, "BTC/USD")
				So(trade.Data[0].Side, ShouldEqual, "buy")
				So(trade.Data[0].Price.String(), ShouldEqual, "50000.1")
				So(trade.Data[0].Qty, ShouldEqual, 0.25)
				So(trade.Data[0].TradeID, ShouldEqual, 42)
				So(trade.Data[0].Timestamp, ShouldEqual, time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC))
			})
		})
	})
}

func TestToMarketTrade(t *testing.T) {
	Convey("Setup", t, func() {
		trade := NewTrade()
		So(trade.UnmarshalJSON([]byte(`{
			"channel":"trade",
			"type":"update",
			"data":[{
				"symbol":"ETH/USD",
				"side":"sell",
				"price":"3000",
				"qty":1.5,
				"ord_type":"limit",
				"trade_id":7,
				"timestamp":"2024-01-02T12:00:00Z"
			}]
		}`)), ShouldBeNil)

		book := market.NewTrade("", "", nil)

		Convey("When ToMarket projects prints", func() {
			trade.ToMarket(book)

			Convey("nomagique trade rows carry decimals", func() {
				So(book.Data, ShouldHaveLength, 1)
				So(book.Data[0].Symbol, ShouldEqual, "ETH/USD")
				So(book.Data[0].Side, ShouldEqual, market.SideSell)
				So(book.Data[0].Price, ShouldEqual, decimal.RequireFromString("3000"))
				So(book.Data[0].Qty.String(), ShouldEqual, "1.5")
				So(book.Data[0].TradeID, ShouldEqual, 7)
			})
		})
	})
}

func TestNewTradeSubscription(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTradeSubscription is marshaled", func() {
			raw, err := sonic.Marshal(NewTradeSubscription([]string{"BTC/USD"}))

			Convey("symbol list is on the wire", func() {
				So(err, ShouldBeNil)
				var frame map[string]any
				So(sonic.Unmarshal(raw, &frame), ShouldBeNil)
				params := frame["params"].(map[string]any)
				So(params["channel"], ShouldEqual, "trade")
				So(params["symbol"], ShouldResemble, []any{"BTC/USD"})
			})
		})
	})
}
