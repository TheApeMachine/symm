package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRelativeMoveNext(t *testing.T) {
	t.Parallel()

	Convey("Given a RelativeMove dynamic", t, func() {
		move := NewRelativeMove()

		Convey("It should return zero change at parity", func() {
			out, err := move.Next(0, 100, 100)

			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, 0, 1e-6)
		})

		Convey("It should return positive change when price rose", func() {
			out, err := move.Next(0, 110, 100)

			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 0)
		})

		Convey("It should return negative change when price fell", func() {
			out, err := move.Next(0, 90, 100)

			So(err, ShouldBeNil)
			So(out, ShouldBeLessThan, 0)
		})
	})
}
