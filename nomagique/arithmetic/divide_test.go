package arithmetic

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDivideNext(t *testing.T) {
	Convey("Given the binary division primitive", t, func() {
		op := NewDivide()

		Convey("it maps each pair to its quotient", func() {
			So(drive[[2]float64, float64](op, &[2]float64{300, 2}), ShouldEqual, 150)
			So(drive[[2]float64, float64](op, &[2]float64{-6, 4}), ShouldEqual, -1.5)
		})

		Convey("a zero divisor has no quotient: no fact is yielded", func() {
			pair := [2]float64{1, 0}
			answers := 0

			for range op.Next(transport.NewOne(unsafe.Pointer(&pair)).Next(nil)) {
				answers++
			}

			So(answers, ShouldEqual, 0)
			So(op.Error(), ShouldBeNil)
		})

		Convey("each arrival maps independently: the primitive holds no state", func() {
			So(drive[[2]float64, float64](op, &[2]float64{8, 2}), ShouldEqual, 4)
			So(drive[[2]float64, float64](op, &[2]float64{8, 2}), ShouldEqual, 4)
		})
	})
}
