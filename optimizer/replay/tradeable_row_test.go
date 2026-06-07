package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeableRow(t *testing.T) {
	Convey("Given an explicit tradeable fixture row", t, func() {
		at := time.Unix(1_700_000_000, 0).UTC()
		measurement := TradeableRow("BTC/EUR", 100, at)

		Convey("It should carry symmetric quotes and walkable depth for tests", func() {
			So(measurement.Bid, ShouldEqual, 100)
			So(measurement.Ask, ShouldEqual, 100)
			So(measurement.HasBookDepth(), ShouldBeTrue)
			So(measurement.At, ShouldEqual, at)
		})
	})
}
