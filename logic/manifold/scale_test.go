package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEpochScaleUpdate(t *testing.T) {
	Convey("Given one batch statistic per event time", t, func() {
		scale := NewEpochScale(time.Second)
		first, ready := scale.Update(2, time.Unix(1, 0))
		second, readyAfter := scale.Update(4, time.Unix(2, 0))

		Convey("It should apply the configured temporal half-life once", func() {
			So(ready, ShouldBeTrue)
			So(readyAfter, ShouldBeTrue)
			So(first, ShouldEqual, 2.0)
			So(second, ShouldEqual, 3.0)
		})
	})
}
