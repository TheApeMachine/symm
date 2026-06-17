package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategoryFromSignalName(testingTB *testing.T) {
	Convey("Given signal-specific classifier indices", testingTB, func() {
		Convey("It should map toxicity categories", func() {
			So(CategoryFromSignalName("toxicity", 2), ShouldEqual, CategoryLiquidityVacuum)
		})

		Convey("It should map exhaust categories", func() {
			So(CategoryFromSignalName("exhaust", 3), ShouldEqual, CategoryThermalExhaustion)
		})

		Convey("It should reject unknown signals", func() {
			So(CategoryFromSignalName("prediction", 1), ShouldEqual, CategoryTypeNone)
		})
	})
}
