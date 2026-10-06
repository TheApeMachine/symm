package linear_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algebra/linear"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestMatrixNext(t *testing.T) {
	Convey("Matrix multiplies a row-major matrix by a vector", t, func() {
		op := linear.NewMatrix(2, 3)
		pair := [2][]float64{{1, 2, 3, 4, 5, 6}, {1, 0, -1}}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{-2, -2})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Matrix records a shape error for a mismatched vector", t, func() {
		op := linear.NewMatrix(2, 3)
		pair := [2][]float64{{1, 2, 3, 4, 5, 6}, {1, 0}}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&pair))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}

func TestTransformNext(t *testing.T) {
	Convey("Transform multiplies each arriving vector by the configured weights", t, func() {
		op := linear.NewTransform(2, 2, []float64{1, 2, 3, 4})
		vec := []float64{1, 1}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&vec))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(out[0], ShouldResemble, []float64{3, 7})
		So(op.Error(), ShouldBeNil)
	})

	Convey("Transform rejects weights that do not fill the configured shape", t, func() {
		op := linear.NewTransform(2, 2, []float64{1, 2, 3})
		vec := []float64{1, 1}
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&vec))
		}
		out := tests.CollectSeq[[]float64](op.Next(in))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
	})
}
