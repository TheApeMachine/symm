package causal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVolumeWindowNext(testingTB *testing.T) {
	testingTB.Parallel()

	Convey("Given a five minute window", testingTB, func() {
		volumeWindow := NewVolumeWindow(5 * time.Minute)
		start := time.Unix(1_700_000_000, 0)

		Convey("It should accumulate within the window", func() {
			sum, err := volumeWindow.Next(0, float64(start.UnixNano()), 10)

			So(err, ShouldBeNil)
			So(sum, ShouldEqual, 10)

			sum, err = volumeWindow.Next(0, float64(start.Add(time.Minute).UnixNano()), 15)

			So(err, ShouldBeNil)
			So(sum, ShouldEqual, 25)
		})

		Convey("It should return the closed sum when the window rolls", func() {
			_, _ = volumeWindow.Next(0, float64(start.UnixNano()), 10, 100)
			_, _ = volumeWindow.Next(0, float64(start.Add(time.Minute).UnixNano()), 15, 100)

			closed, err := volumeWindow.Next(0, float64(start.Add(6*time.Minute).UnixNano()), 5, 110)

			So(err, ShouldBeNil)
			So(closed, ShouldEqual, 25)
			So(volumeWindow.Sum(), ShouldEqual, 5)
			So(volumeWindow.Anchor(), ShouldEqual, 110)
		})
	})
}
