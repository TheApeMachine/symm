package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBelowMedianNext(t *testing.T) {
	t.Parallel()

	Convey("Given a BelowMedian gate", t, func() {
		gate := NewBelowMedian()

		Convey("It should pass the low outlier", func() {
			out, err := gate.Next(10, 100, 90, 80)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 10)
		})

		Convey("It should zero values at or above the median", func() {
			out, err := gate.Next(100, 10, 90, 80)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})
	})
}
