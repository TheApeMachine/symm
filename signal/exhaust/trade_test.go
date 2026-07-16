package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeOn(testingTB *testing.T) {
	Convey("Given an exhaust trade ingestor", testingTB, func() {
		trade := &Trade{cache: tradeCache()}
		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.5,"qty":1.25,"ord_type":"market","trade_id":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a trade frame arrives", func() {
			trade.On(payload)

			Convey("Then trade rows should accumulate in cache", func() {
				So(len(tradeRows(trade.cache)), ShouldEqual, 1)
				So(tradeRows(trade.cache)[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}
