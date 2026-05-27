package order

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLimitBuyBid(t *testing.T) {
	Convey("Given a bid-side limit entry", t, func() {
		request := LimitBuyBid("BTC/EUR", 10, 49999, "token")

		Convey("It should post a limit buy at the bid", func() {
			So(request.Method, ShouldEqual, MethodAddOrder)
			So(request.Params.OrderType, ShouldEqual, Limit)
			So(request.Params.LimitPrice, ShouldEqual, 49999)
			So(request.Params.CashOrderQty, ShouldEqual, 10)
		})
	})
}

func TestCancelOrder(t *testing.T) {
	Convey("Given a resting order id", t, func() {
		request := CancelOrder("ORDER-1", "token")

		Convey("It should build cancel_order", func() {
			So(request.Method, ShouldEqual, MethodCancelOrder)
			So(request.Params.OrderID, ShouldEqual, "ORDER-1")
		})
	})
}
