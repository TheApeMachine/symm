package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestActionKrakenOrderType(t *testing.T) {
	Convey("Given playbook action types", t, func() {
		marketType, marketErr := ActionMarket.KrakenOrderType()
		settleType, settleErr := ActionSettlePosition.KrakenOrderType()

		Convey("It should map to Kraken order types", func() {
			So(marketErr, ShouldBeNil)
			So(settleErr, ShouldBeNil)
			So(marketType, ShouldEqual, OrderMarket)
			So(settleType, ShouldEqual, OrderSettlePosition)
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
