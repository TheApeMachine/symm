package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeFinite(t *testing.T) {
	Convey("Given finite normalization helpers", t, func() {
		Convey("Then finite values are retained and zero stays explicit", func() {
			So(*NormalizeFinite(0.5), ShouldEqual, 0.5)
			So(NormalizeFinite(0), ShouldBeNil)
			So(NormalizeRatio(10, 5), ShouldNotBeNil)
			So(*NormalizeRatio(10, 5), ShouldEqual, 2)
			So(*NormalizeDeviation(15, 5), ShouldEqual, 2)
			So(*NormalizeSigned(-0.25), ShouldEqual, -0.25)
		})
	})
}
