package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarketRegimeString(t *testing.T) {
	Convey("Given market regime values", t, func() {
		Convey("It should map them to stable feedback bucket names", func() {
			So(RegimeUnknown.String(), ShouldEqual, "")
			So(RegimeChoppy.String(), ShouldEqual, "choppy")
			So(RegimeBullish.String(), ShouldEqual, "bullish")
		})
	})
}

func TestFeedbackRegime(t *testing.T) {
	Convey("Given a perspective with a market regime", t, func() {
		perspective := Perspective{Regime: RegimeChoppy}
		measurement := Measurement{Regime: "microstructure"}

		Convey("It should prefer the market regime over the source-local label", func() {
			So(FeedbackRegime(perspective, measurement), ShouldEqual, "choppy")
		})
	})

	Convey("Given a perspective without a market regime", t, func() {
		measurement := Measurement{Regime: "fluid"}

		Convey("It should preserve the signal label", func() {
			So(FeedbackRegime(Perspective{}, measurement), ShouldEqual, "fluid")
		})
	})
}
