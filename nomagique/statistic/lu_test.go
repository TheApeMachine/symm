package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestSolveLUNext(t *testing.T) {
	Convey("Given a SolveLU primitive", t, func() {
		op := NewSolveLU()

		Convey("solves a well-conditioned system", func() {
			system := [2][]float64{{2, 1, 1, 3}, {3, 5}}
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][2][]float64{system})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(len(out[0]), ShouldEqual, 2)
			So(out[0][0], ShouldAlmostEqual, 0.8, 1e-12)
			So(out[0][1], ShouldAlmostEqual, 1.4, 1e-12)
		})

		Convey("yields an empty solution for a singular system", func() {
			system := [2][]float64{{1, 2, 2, 4}, {1, 2}}
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][2][]float64{system})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			So(len(out[0]), ShouldEqual, 0)
		})

		Convey("nil arrival records ErrShape", func() {
			fresh := NewSolveLU()
			out := tests.CollectSeq[[]float64](fresh.Next(func(yield func(unsafe.Pointer) bool) { yield(nil) }))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}

func TestInvertLUNext(t *testing.T) {
	Convey("Given an InvertLU primitive", t, func() {
		op := NewInvertLU()

		Convey("inverts a non-singular matrix", func() {
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][]float64{{4, 7, 2, 6}})))

			So(op.Error(), ShouldBeNil)
			So(len(out[0]), ShouldEqual, 4)
			So(out[0][0], ShouldAlmostEqual, 0.6, 1e-12)
			So(out[0][1], ShouldAlmostEqual, -0.7, 1e-12)
			So(out[0][2], ShouldAlmostEqual, -0.2, 1e-12)
			So(out[0][3], ShouldAlmostEqual, 0.4, 1e-12)
		})

		Convey("yields an empty inverse for a singular matrix", func() {
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][]float64{{1, 2, 2, 4}})))

			So(op.Error(), ShouldBeNil)
			So(len(out[0]), ShouldEqual, 0)
		})

		Convey("a non-square arrival records ErrShape", func() {
			fresh := NewInvertLU()
			out := tests.CollectSeq[[]float64](fresh.Next(tests.SliceToSeq([][]float64{{1, 2, 3}})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
