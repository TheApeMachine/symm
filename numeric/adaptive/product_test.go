package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProductNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Product dynamic", t, func() {
		product := NewProduct()

		Convey("It should multiply operands together", func() {
			out, err := product.Next(2, 0.5, 0.4)

			So(err, ShouldBeNil)
			So(out, ShouldAlmostEqual, 0.4, 1e-9)
		})

		Convey("It should zero when any operand is non-positive", func() {
			out, err := product.Next(2, 0, 0.5)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})
	})
}
