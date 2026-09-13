package arithmetic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubtractNext(t *testing.T) {
	Convey("Given the binary subtraction primitive", t, func() {
		op := NewSubtract()

		Convey("it maps each pair to its difference", func() {
			So(drive[[2]float64, float64](op, &[2]float64{5, 3}), ShouldEqual, 2)
			So(drive[[2]float64, float64](op, &[2]float64{3, 5}), ShouldEqual, -2)
			So(drive[[2]float64, float64](op, &[2]float64{0, 0}), ShouldEqual, 0)
		})

		Convey("each arrival maps independently: the primitive holds no state", func() {
			So(drive[[2]float64, float64](op, &[2]float64{1, 1}), ShouldEqual, 0)
			So(drive[[2]float64, float64](op, &[2]float64{1, 1}), ShouldEqual, 0)
		})
	})
}
