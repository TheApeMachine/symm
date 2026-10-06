package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCausalResidualNext(t *testing.T) {
	Convey("Given a CausalResidual primitive", t, func() {
		op := NewCausalResidual()

		Convey("evaluates residuals against prior moments", func() {
			est := NewEstimator()
			var readings [][10]float64

			for out := range est.Next(tests.SliceToSeq([]float64{10.0, 20.0})) {
				readings = append(readings, *(*[10]float64)(out))
			}

			out := tests.CollectSeq[[8]float64](op.Next(tests.SliceToSeq(readings)))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 2)

			// First reading: no prior
			So(out[0][0], ShouldEqual, 0)
			So(out[0][1], ShouldEqual, 10.0)
			So(out[0][4], ShouldEqual, 0.0)

			// Second reading: has prior from first reading
			So(out[1][0], ShouldEqual, 1)
			So(out[1][1], ShouldEqual, 10.0)
			So(out[1][4], ShouldEqual, 10.0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewCausalResidual()
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[[8]float64](fresh.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("handles early consumer termination", func() {
			fresh := NewCausalResidual()
			est := NewEstimator()
			var readings [][10]float64

			for out := range est.Next(tests.SliceToSeq([]float64{10.0, 20.0})) {
				readings = append(readings, *(*[10]float64)(out))
			}

			count := 0

			for range fresh.Next(tests.SliceToSeq(readings)) {
				count++
				break
			}

			So(count, ShouldEqual, 1)
			So(fresh.Error(), ShouldBeNil)
		})
	})
}
