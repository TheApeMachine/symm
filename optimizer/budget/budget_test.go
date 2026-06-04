package budget

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDeriveMeasurementSampleCap(t *testing.T) {
	convey.Convey("Given a large capture file row count", t, func() {
		cap := DeriveMeasurementSampleCap(1000000, 8)

		convey.Convey("It should subsample with sqrt scaling", func() {
			convey.So(cap, convey.ShouldBeLessThan, 1000000)
			convey.So(cap, convey.ShouldBeGreaterThan, 1000)
		})
	})

	convey.Convey("Given a small row count under the worker-square floor", t, func() {
		convey.So(DeriveMeasurementSampleCap(10, 8), convey.ShouldEqual, 10)
	})
}
