package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnchorChange(t *testing.T) {
	Convey("Given a positive move off the anchor", t, func() {
		move, magnitude := AnchorChange(100, 110)

		Convey("It should return signed and absolute forms", func() {
			So(move, ShouldAlmostEqual, 0.1, 1e-9)
			So(magnitude, ShouldAlmostEqual, 0.1, 1e-9)
		})
	})

	Convey("Given a zero anchor", t, func() {
		move, magnitude := AnchorChange(0, 10)

		Convey("It should return zero", func() {
			So(move, ShouldEqual, 0)
			So(magnitude, ShouldEqual, 0)
		})
	})
}
