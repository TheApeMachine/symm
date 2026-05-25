package learned

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSampleRatioPenalizesLosingPredictions(t *testing.T) {
	Convey("Given a positive predicted return", t, func() {
		Convey("It should accept zero actual return as a losing sample", func() {
			sample, ok := SampleRatio(0.01, 0)

			So(ok, ShouldBeTrue)
			So(sample, ShouldEqual, 1)
		})

		Convey("It should accept negative actual return as a losing sample", func() {
			sample, ok := SampleRatio(0.01, -0.005)

			So(ok, ShouldBeTrue)
			So(sample, ShouldAlmostEqual, 0.5, 0.0001)
		})

		Convey("It should scale winning samples by actual/predicted", func() {
			sample, ok := SampleRatio(0.01, 0.005)

			So(ok, ShouldBeTrue)
			So(sample, ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
