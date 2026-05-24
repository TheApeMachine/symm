package order

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestMarketBuyCashIncludesConditionalStop(t *testing.T) {
	convey.Convey("Given a EUR market buy with stop", t, func() {
		request := MarketBuyCash("BTC/EUR", 10, 95000, 94900, "token")

		convey.Convey("It should use add_order with cash_order_qty and OTO stop", func() {
			convey.So(request.Method, convey.ShouldEqual, MethodAddOrder)
			convey.So(request.Params.OrderType, convey.ShouldEqual, Market)
			convey.So(request.Params.Side, convey.ShouldEqual, Buy)
			convey.So(request.Params.CashOrderQty, convey.ShouldEqual, 10)
			convey.So(request.Params.Conditional, convey.ShouldNotBeNil)
			convey.So(request.Params.Conditional.OrderType, convey.ShouldEqual, StopLossLimit)
			convey.So(request.Params.Conditional.TriggerPrice, convey.ShouldEqual, 95000)
		})
	})
}

func TestParseAck(t *testing.T) {
	convey.Convey("Given an add_order success frame", t, func() {
		payload := []byte(`{
			"method": "add_order",
			"req_id": 42,
			"success": true,
			"result": {"order_id": "ORDER-1"}
		}`)

		convey.Convey("It should parse the order id", func() {
			ack, err := ParseAck(payload)
			convey.So(err, convey.ShouldBeNil)
			convey.So(ack.Success, convey.ShouldBeTrue)
			convey.So(ack.Result.OrderID, convey.ShouldEqual, "ORDER-1")
		})
	})
}

func TestParseExecutionFills(t *testing.T) {
	convey.Convey("Given an executions trade frame", t, func() {
		payload := []byte(`{
			"channel": "executions",
			"type": "update",
			"data": [{
				"order_id": "ORDER-1",
				"symbol": "BTC/EUR",
				"side": "buy",
				"exec_type": "trade",
				"last_qty": 0.001,
				"last_price": 95000
			}]
		}`)

		convey.Convey("It should extract one fill", func() {
			fills, err := ParseExecutionFills(payload)
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(fills), convey.ShouldEqual, 1)
			convey.So(fills[0].OrderID, convey.ShouldEqual, "ORDER-1")
			convey.So(fills[0].Price, convey.ShouldEqual, 95000)
		})
	})
}
