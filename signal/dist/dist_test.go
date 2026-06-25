package dist

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestWrite(t *testing.T) {
	Convey("Given category shares", t, func() {
		measurement := datura.Acquire("test", datura.APPJSON)

		defer measurement.Release()

		confidence := Write(measurement, []Share{
			{Key: "ignition", Category: logic.CategoryVerticalIgnition, Mass: 3},
			{Key: "compression", Category: logic.CategoryCoiledCompression, Mass: 1},
		})

		Convey("It should publish a normalised distribution and peak confidence", func() {
			So(confidence, ShouldAlmostEqual, 0.75, 1e-12)
			So(datura.Peek[float64](measurement, "output", "ignition"), ShouldAlmostEqual, 0.75, 1e-12)
			So(
				datura.Peek[float64](measurement, "output", fmt.Sprintf("category.%d", logic.CategoryIndex(logic.CategoryVerticalIgnition))),
				ShouldAlmostEqual,
				0.75,
				1e-12,
			)
			So(
				datura.Peek[float64](measurement, "output", "value"),
				ShouldEqual,
				float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)),
			)
		})
	})
}

func TestWriteAggregatesDuplicateCategories(t *testing.T) {
	Convey("Given two evidence keys for one category", t, func() {
		measurement := datura.Acquire("test", datura.APPJSON)

		defer measurement.Release()

		confidence := Write(measurement, []Share{
			{Key: "trend", Category: logic.CategoryOrganicTrend, Mass: 2},
			{Key: "steady", Category: logic.CategoryOrganicTrend, Mass: 1},
			{Key: "ignition", Category: logic.CategoryVerticalIgnition, Mass: 1},
		})

		Convey("It should preserve keys and publish the summed category mass", func() {
			So(confidence, ShouldAlmostEqual, 0.75, 1e-12)
			So(datura.Peek[float64](measurement, "output", "trend"), ShouldAlmostEqual, 0.5, 1e-12)
			So(datura.Peek[float64](measurement, "output", "steady"), ShouldAlmostEqual, 0.25, 1e-12)
			So(
				datura.Peek[float64](measurement, "output", fmt.Sprintf("category.%d", logic.CategoryIndex(logic.CategoryOrganicTrend))),
				ShouldAlmostEqual,
				0.75,
				1e-12,
			)
		})
	})
}
