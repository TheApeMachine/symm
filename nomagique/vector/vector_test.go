package vector_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/vector"
)

func TestVectorOperations(t *testing.T) {
	Convey("Vector Sum", t, func() {
		sum := vector.NewSum()
		pair := vector.Pair{
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
		diff := vector.NewDifference()
		pair := vector.Pair{
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
		dot := vector.NewDot()
		pair := vector.Pair{
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
		scale := vector.NewScale()
		input := vector.ScaleInput{
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
		finite := vector.NewFinite()
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
		design := vector.NewDesign(1, 0)
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
