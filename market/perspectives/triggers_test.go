package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestProtectiveTriggerMath(t *testing.T) {
	Convey("The shared protective-trigger math", t, func() {
		Convey("A stop rests below entry and fires on the way down", func() {
			level := ProtectiveLevel(ActionStopLoss, 100, 0, 0.02)
			So(level, ShouldAlmostEqual, 98, 1e-9)
			So(ProtectiveBreached(ActionStopLoss, level, 97.9), ShouldBeTrue)
			So(ProtectiveBreached(ActionStopLoss, level, 98.1), ShouldBeFalse)
		})

		Convey("A take-profit rests above entry and fires on the way up", func() {
			level := ProtectiveLevel(ActionTakeProfit, 100, 0, 0.03)
			So(level, ShouldAlmostEqual, 103, 1e-9)
			So(ProtectiveBreached(ActionTakeProfit, level, 103.1), ShouldBeTrue)
			So(ProtectiveBreached(ActionTakeProfit, level, 102.9), ShouldBeFalse)
		})

		Convey("A trailing stop measures from the peak", func() {
			level := ProtectiveLevel(ActionTrailingStop, 0, 120, 0.02)
			So(level, ShouldAlmostEqual, 117.6, 1e-9)
			So(ProtectiveBreached(ActionTrailingStop, level, 117.5), ShouldBeTrue)
		})

		Convey("Short protective triggers invert long levels", func() {
			stop := ProtectiveLevelForSide(trading.Sell, ActionStopLoss, 100, 0, 0.02)
			So(stop, ShouldAlmostEqual, 102, 1e-9)
			So(ProtectiveBreachedForSide(trading.Sell, ActionStopLoss, stop, 102.1), ShouldBeTrue)

			take := ProtectiveLevelForSide(trading.Sell, ActionTakeProfit, 100, 0, 0.03)
			So(take, ShouldAlmostEqual, 97, 1e-9)
			So(ProtectiveBreachedForSide(trading.Sell, ActionTakeProfit, take, 96.9), ShouldBeTrue)
		})

		Convey("The per-node offset overrides the global, but nonsensical fractions fall back", func() {
			So(TriggerOffset(0.02, 0.05), ShouldEqual, 0.02)
			So(TriggerOffset(0, 0.05), ShouldEqual, 0.05)
			So(TriggerOffset(1.5, 0.05), ShouldEqual, 0.05) // >=1 is nonsensical
			So(TriggerOffset(-0.3, 0.05), ShouldEqual, 0.05)
		})

		Convey("Limit variants rest as makers; trailing/protective classification holds", func() {
			So(ExitRestsAsLimit(ActionStopLossLimit), ShouldBeTrue)
			So(ExitRestsAsLimit(ActionStopLoss), ShouldBeFalse)
			So(IsTrailingExit(ActionTrailingStop), ShouldBeTrue)
			So(IsTrailingExit(ActionStopLoss), ShouldBeFalse)
			So(IsProtectiveExit(ActionStopLoss), ShouldBeTrue)
			So(IsProtectiveExit(ActionMarket), ShouldBeFalse)
			So(IsProtectiveExit(ActionSettlePosition), ShouldBeFalse)
		})
	})
}
