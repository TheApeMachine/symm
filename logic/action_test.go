package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestActionKrakenOrderType(t *testing.T) {
	Convey("Given playbook action types", t, func() {
		marketType, marketErr := ActionMarket.KrakenOrderType()
		settleType, settleErr := ActionSettlePosition.KrakenOrderType()
		stopType, stopErr := ActionStopLoss.KrakenOrderType()
		takeProfitType, takeProfitErr := ActionTakeProfit.KrakenOrderType()
		trailingType, trailingErr := ActionTrailingStop.KrakenOrderType()

		Convey("It should map to Kraken order types", func() {
			So(marketErr, ShouldBeNil)
			So(settleErr, ShouldBeNil)
			So(stopErr, ShouldBeNil)
			So(takeProfitErr, ShouldBeNil)
			So(trailingErr, ShouldBeNil)
			So(marketType, ShouldEqual, OrderMarket)
			So(settleType, ShouldEqual, OrderSettlePosition)
			So(stopType, ShouldEqual, OrderStopLoss)
			So(takeProfitType, ShouldEqual, OrderTakeProfit)
			So(trailingType, ShouldEqual, OrderTrailingStop)
			So(stopType, ShouldNotEqual, OrderMarket)
			So(takeProfitType, ShouldNotEqual, OrderMarket)
			So(trailingType, ShouldNotEqual, OrderMarket)
		})
	})
}

func TestActionIsExit(t *testing.T) {
	Convey("Given settle and market actions", t, func() {
		Convey("It should classify exits correctly", func() {
			So(ActionSettlePosition.IsExit(), ShouldBeTrue)
			So(ActionMarket.IsExit(), ShouldBeFalse)
		})
	})
}
