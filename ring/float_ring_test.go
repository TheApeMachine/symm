package ring

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFloatRingMeanStdDev(t *testing.T) {
	Convey("Given a ring of spreads", t, func() {
		sampleRing := NewFloatRing(4)
		sampleRing.Push(0.1)
		sampleRing.Push(0.2)
		sampleRing.Push(0.3)
		sampleRing.Push(0.4)

		mean, stddev := sampleRing.MeanStdDev()

		Convey("It should compute mean and standard deviation", func() {
			So(mean, ShouldAlmostEqual, 0.25, 1e-9)
			So(stddev, ShouldBeGreaterThan, 0)
		})
	})
}
