package arithmetic

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive evaluates one scalar payload through one primitive.
*/
func drive[From, To any](op core.Primitive, payload *From) To {
	var answer To

	for out := range op.Next(transport.NewOne(unsafe.Pointer(payload)).Next(nil)) {
		answer = *(*To)(out)
	}

	return answer
}

func TestAddNext(t *testing.T) {
	Convey("Given the binary addition primitive", t, func() {
		op := NewAdd()

		Convey("it maps each pair to its sum", func() {
			So(drive[[2]float64, float64](op, &[2]float64{2, 3}), ShouldEqual, 5)
			So(drive[[2]float64, float64](op, &[2]float64{-2, 3}), ShouldEqual, 1)
			So(drive[[2]float64, float64](op, &[2]float64{0, 0}), ShouldEqual, 0)
		})

		Convey("each arrival maps independently: the primitive holds no total", func() {
			So(drive[[2]float64, float64](op, &[2]float64{1, 1}), ShouldEqual, 2)
			So(drive[[2]float64, float64](op, &[2]float64{1, 1}), ShouldEqual, 2)
		})

		Convey("IEEE exceptional operands flow through per IEEE-754", func() {
			So(drive[[2]float64, float64](op, &[2]float64{math.Inf(1), 1}), ShouldEqual, math.Inf(1))
			So(math.IsNaN(drive[[2]float64, float64](op, &[2]float64{math.Inf(1), math.Inf(-1)})), ShouldBeTrue)
			So(math.IsNaN(drive[[2]float64, float64](op, &[2]float64{math.NaN(), 5})), ShouldBeTrue)
		})
	})
}
