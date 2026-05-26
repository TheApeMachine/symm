package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPositiveMoveNext(t *testing.T) {
	t.Parallel()

	Convey("Given a PositiveMove with a one-tenth-percent bound", t, func() {
		move := NewPositiveMove(0.001)

		Convey("It should return zero when price has not risen", func() {
			out, err := move.Next(0, 1, 1)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})

		Convey("It should ramp linearly below the bound", func() {
			out, err := move.Next(0, 1.0005, 1)

			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, 0.5, 1e-6)
		})

		Convey("It should map excess above the bound into (0, 1)", func() {
			out, err := move.Next(0, 1.003, 1)

			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 0.5)
			So(out, ShouldBeLessThan, 1)
		})
	})
}
