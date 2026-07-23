package fluid

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWireRow(t *testing.T) {
	Convey("Given a finite field row", t, func() {
		row := WireRow(map[string]any{
			"symbol": "BTC/USD",
			"re":     12.5,
			"div":    0.01,
		})

		Convey("It should pass through unchanged", func() {
			So(row, ShouldNotBeNil)
			So(row["re"], ShouldEqual, 12.5)
		})
	})

	Convey("Given a row with non-finite reynolds", t, func() {
		row := WireRow(map[string]any{
			"symbol": "BTC/USD",
			"re":     math.Inf(1),
		})

		Convey("It should reject the row", func() {
			So(row, ShouldBeNil)
		})
	})
}
