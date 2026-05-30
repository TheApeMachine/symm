package market

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDepthVisibleNotionalFraction(t *testing.T) {
	convey.Convey("Given depth that fully covers notional", t, func() {
		levels := []BookLevel{{Price: 100, Qty: 2}}
		fraction := DepthVisibleNotionalFraction(levels, 150)

		convey.Convey("It should report full visible coverage", func() {
			convey.So(fraction, convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given depth that only partially covers notional", t, func() {
		levels := []BookLevel{{Price: 100, Qty: 0.5}}
		fraction := DepthVisibleNotionalFraction(levels, 150)

		convey.Convey("It should report partial coverage", func() {
			convey.So(fraction, convey.ShouldAlmostEqual, 50.0/150.0, 0.001)
		})
	})
}
