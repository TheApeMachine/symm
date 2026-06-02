package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeUpdateSetEnvelopeType(t *testing.T) {
	Convey("Given a trade update", t, func() {
		trade := TradeUpdate{Symbol: "BTC/EUR", Price: 100, Qty: 1}

		trade.SetEnvelopeType("snapshot")

		Convey("It should record the envelope type", func() {
			So(trade.Type, ShouldEqual, "snapshot")
		})
	})
}
