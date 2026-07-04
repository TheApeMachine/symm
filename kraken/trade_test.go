package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewTradeDataSlice(t *testing.T) {
	Convey("Given Kraken trade data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "MATIC/USD",
			"side": "buy",
			"price": 0.5147,
			"qty": 6423.46326,
			"ord_type": "limit",
			"trade_id": 4665846,
			"timestamp": "2023-09-25T07:48:36.925533Z"
		}]`)

		trades := NewTradeDataSlice(payload)

		Convey("It should decode the trade fields", func() {
			So(len(trades), ShouldEqual, 1)

			trade := trades[0]

			So(trade.Symbol, ShouldEqual, "MATIC/USD")
			So(trade.OrderType, ShouldEqual, "limit")
			So(trade.TradeID, ShouldEqual, 4665846)
			So(trade.Price, ShouldAlmostEqual, 0.5147)
			So(trade.Timestamp.IsZero(), ShouldBeFalse)
		})
	})
}
