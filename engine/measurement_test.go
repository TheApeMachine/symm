package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementAnchorPrice(t *testing.T) {
	Convey("Given a measurement with quote fields", t, func() {
		Convey("It should prefer Last over the mid", func() {
			So(Measurement{Last: 100, Bid: 99, Ask: 101}.AnchorPrice(), ShouldEqual, 100)
		})

		Convey("It should fall back to the quote mid when Last is missing", func() {
			So(Measurement{Bid: 99, Ask: 101}.AnchorPrice(), ShouldEqual, 100)
		})

		Convey("It should reject incomplete quotes", func() {
			So(Measurement{Bid: 99}.AnchorPrice(), ShouldEqual, 0)
			So(Measurement{}.AnchorPrice(), ShouldEqual, 0)
		})
	})
}
