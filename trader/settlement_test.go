package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSpotSettlementMatchesLiveFeeBasis(t *testing.T) {
	convey.Convey("Given one spot fill", t, func() {
		baseQty := 0.00123456
		fillPrice := 95123.45
		feePct := 0.26

		proceeds := spotProceedsEUR(baseQty, fillPrice)
		fee := spotTakerFeeEUR(proceeds, feePct)

		convey.Convey("It should derive fees from proceeds not requested notional", func() {
			convey.So(proceeds, convey.ShouldAlmostEqual, baseQty*fillPrice, 0.0001)
			convey.So(fee, convey.ShouldAlmostEqual, proceeds*feePct/100, 0.0001)
		})
	})
}

func TestStopLossLimitFill(t *testing.T) {
	convey.Convey("Given a triggered stop-loss-limit", t, func() {
		trigger := 95.0
		limit := StopLimitBelow(trigger)

		convey.Convey("It should not fill above the trigger", func() {
			fill := StopLossLimitFill(96, trigger, limit, 95.9, 96.1, 1, nil)
			convey.So(fill, convey.ShouldEqual, 0)
		})

		convey.Convey("It should respect the limit floor on gap down", func() {
			fill := StopLossLimitFill(90, trigger, limit, 89.9, 90.1, 1, nil)
			convey.So(fill, convey.ShouldAlmostEqual, limit, 0.0001)
		})
	})
}
