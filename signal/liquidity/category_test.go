package liquidity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestLiquidityReading(t *testing.T) {
	Convey("Given peer quote volumes", t, func() {
		peers := []float64{10, 20, 30, 40, 50}

		category, clarity, standout, err := liquidityReading(45, peers)

		Convey("It should classify robust liquidity", func() {
			So(err, ShouldBeNil)
			So(category, ShouldEqual, perspectives.CategoryRobustLiquidity)
			So(clarity, ShouldBeGreaterThan, 0)
			So(standout, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})

	Convey("Given extreme scarcity", t, func() {
		category, _, _, err := liquidityReading(5, []float64{10, 20, 30, 40})

		Convey("It should classify extreme scarcity", func() {
			So(err, ShouldBeNil)
			So(category, ShouldEqual, perspectives.CategoryExtremeScarcity)
		})
	})
}
