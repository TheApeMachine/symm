package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRequireRealCategoryIndex(t *testing.T) {
	Convey("Given a four-category signal", t, func() {
		Convey("It should accept indices in [1,4]", func() {
			So(RequireRealCategoryIndex(1, 4), ShouldBeNil)
			So(RequireRealCategoryIndex(4, 4), ShouldBeNil)
		})

		Convey("It should reject none and out-of-range indices", func() {
			So(RequireRealCategoryIndex(0, 4), ShouldNotBeNil)
			So(RequireRealCategoryIndex(5, 4), ShouldNotBeNil)
		})
	})
}

func TestCategoryIndexMaps(t *testing.T) {
	Convey("Given canonical source maps", t, func() {
		Convey("CVD categories should map to 1..4", func() {
			So(CategoryIndexFor(SourceCVD, CategoryHiddenAbsorption), ShouldEqual, 1)
			So(CategoryIndexFor(SourceCVD, CategoryAggressiveDrive), ShouldEqual, 2)
			So(CategoryIndexFor(SourceCVD, CategoryStochasticBalance), ShouldEqual, 3)
			So(CategoryIndexFor(SourceCVD, CategoryVolumeStarvation), ShouldEqual, 4)
		})

		Convey("None categories should map to zero", func() {
			So(CategoryIndexFor(SourceCVD, CategoryTypeNone), ShouldEqual, CategoryNoneIndex)
		})
	})
}
