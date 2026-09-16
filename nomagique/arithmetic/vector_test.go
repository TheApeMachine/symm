package arithmetic_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	arithmetic "github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestVectorOperations(t *testing.T) {
	Convey("Vector Sum", t, func() {
		sum := arithmetic.NewSum()
		pair := arithmetic.Pair{
			Left:  []float64{1.0, 2.0, 3.0},
			Right: []float64{4.0, 5.0, 6.0},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair))
		}
		out := tests.CollectSeq[[]float64](sum.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{5.0, 7.0, 9.0})
		So(sum.Error(), ShouldBeNil)
	})

	Convey("Vector Difference", t, func() {
		diff := arithmetic.NewDifference()
		pair := arithmetic.Pair{
			Left:  []float64{10.0, 20.0},
			Right: []float64{3.0, 5.0},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair))
		}
		out := tests.CollectSeq[[]float64](diff.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{7.0, 15.0})
		So(diff.Error(), ShouldBeNil)
	})

	Convey("Vector Dot", t, func() {
		dot := arithmetic.NewDot()
		pair := arithmetic.Pair{
			Left:  []float64{1.0, 2.0, 3.0},
			Right: []float64{4.0, 5.0, 6.0},
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair))
		}
		out := tests.CollectSeq[float64](dot.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldEqual, 1*4+2*5+3*6) // 32
		So(dot.Error(), ShouldBeNil)
	})

	Convey("Vector Scale", t, func() {
		scale := arithmetic.NewScale()
		input := arithmetic.ScaleInput{
			Values: []float64{2.0, 4.0, 6.0},
			Factor: 2.5,
		}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&input))
		}
		out := tests.CollectSeq[[]float64](scale.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{5.0, 10.0, 15.0})
		So(scale.Error(), ShouldBeNil)
	})

	Convey("Vector Finite", t, func() {
		finite := arithmetic.NewFinite()
		v := []float64{1.0, 2.0, 3.0}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&v))
		}
		out := tests.CollectSeq[bool](finite.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldBeTrue)
		So(finite.Error(), ShouldBeNil)
	})

	Convey("Vector Design", t, func() {
		design := arithmetic.NewDesign(1, 0)
		v := []float64{10.0, 20.0, 30.0}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&v))
		}
		out := tests.CollectSeq[[]float64](design.Next(in))
		So(len(out), ShouldEqual, 1)
		So(out[0], ShouldResemble, []float64{1.0, 20.0, 10.0})
		So(design.Error(), ShouldBeNil)
	})
}
