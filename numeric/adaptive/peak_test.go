package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeakNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Peak dynamic", t, func() {
		peak := NewPeak()

		Convey("It should pass through the largest value", func() {
			out, err := peak.Next(3, 1, 2)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 3)
		})

		Convey("It should zero a non-peak candidate", func() {
			out, err := peak.Next(2, 3, 1)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})

		Convey("It should reject non-positive out", func() {
			out, err := peak.Next(0, 1)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})
	})
}
