package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestNewTradesHistoryFromMap(t *testing.T) {
	Convey("Given the raw JSON emitted by `kraken paper history --output json`", t, func() {
		raw := []byte(`{
			"trades":[
				{
					"cost":69.95073891625616,
					"fee":0.181871921182266,
					"id":"PAPER-00014",
					"order_id":"PAPER-00013",
					"pair":"AVNTUSD",
					"price":0.0994,
					"side":"buy",
					"status":"filled",
					"time":"2026-07-11T01:06:46.531520+00:00",
					"volume":703.7297677691766
				},
				{
					"cost":70.61928219563687,
					"fee":0.18361013370865584,
					"id":"PAPER-00022",
					"order_id":"PAPER-00021",
					"pair":"AVNTUSD",
					"price":0.10035,
					"side":"sell",
					"status":"filled",
					"time":"2026-07-11T02:08:41.078909+00:00",
					"volume":703.7297677691766
				}
			]
		}`)

		model := datura.Map[any]{}
		So(sonic.Unmarshal(raw, &model), ShouldBeNil)

		Convey("When it is converted into a TradesHistory", func() {
			history := NewTradesHistoryFromMap(model)

			Convey("Then every trade is parsed without panicking on shape or field mismatches", func() {
				So(history.Result.Trades, ShouldHaveLength, 2)

				buy := history.Result.Trades["PAPER-00014"]
				So(buy.OrderID, ShouldEqual, "PAPER-00013")
				So(buy.Pair, ShouldEqual, "AVNTUSD")
				So(buy.Type, ShouldEqual, "buy")
				So(buy.OrderType, ShouldEqual, "market")
				So(buy.Price.Float64(), ShouldEqual, 0.0994)
				So(buy.Cost.Float64(), ShouldEqual, 69.95073891625616)
				So(buy.Fee.Float64(), ShouldEqual, 0.181871921182266)
				So(buy.Volume.Float64(), ShouldEqual, 703.7297677691766)
				So(buy.Time.Float64(), ShouldEqual, 1783732006.53152)

				sell := history.Result.Trades["PAPER-00022"]
				So(sell.Type, ShouldEqual, "sell")
			})
		})
	})
}
