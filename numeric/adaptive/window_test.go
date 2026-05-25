package adaptive

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWindowNext(t *testing.T) {
	t.Parallel()

	Convey("Given a five minute window", t, func() {
		window := NewWindow(5 * time.Minute)
		start := time.Unix(1_700_000_000, 0)

		Convey("It should accumulate within the window", func() {
			sum, err := window.Next(0, float64(start.UnixNano()), 10)

			So(err, ShouldBeNil)
			So(sum, ShouldEqual, 10)

			sum, err = window.Next(0, float64(start.Add(time.Minute).UnixNano()), 15)

			So(err, ShouldBeNil)
			So(sum, ShouldEqual, 25)
		})

		Convey("It should return the closed sum when the window rolls", func() {
			_, _ = window.Next(0, float64(start.UnixNano()), 10, 100)
			_, _ = window.Next(0, float64(start.Add(time.Minute).UnixNano()), 15, 100)

			closed, err := window.Next(0, float64(start.Add(6*time.Minute).UnixNano()), 5, 110)

			So(err, ShouldBeNil)
			So(closed, ShouldEqual, 25)
			So(window.Sum(), ShouldEqual, 5)
			So(window.Anchor(), ShouldEqual, 110)
		})
	})
}
