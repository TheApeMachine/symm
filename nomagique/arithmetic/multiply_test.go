package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMultiplyNext(t *testing.T) {
	Convey("Given the binary multiplication primitive", t, func() {
		op := NewMultiply()

		Convey("it maps each pair to its product", func() {
			So(drive[[2]float64, float64](op, &[2]float64{100, 2}), ShouldEqual, 200)
			So(drive[[2]float64, float64](op, &[2]float64{-3, 4}), ShouldEqual, -12)
			So(drive[[2]float64, float64](op, &[2]float64{0, 5}), ShouldEqual, 0)
		})

		Convey("each arrival maps independently: the primitive holds no state", func() {
			So(drive[[2]float64, float64](op, &[2]float64{2, 2}), ShouldEqual, 4)
			So(drive[[2]float64, float64](op, &[2]float64{2, 2}), ShouldEqual, 4)
		})
	})
}
