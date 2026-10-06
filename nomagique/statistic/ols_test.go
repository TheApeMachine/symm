package statistic

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestOLSNext(t *testing.T) {
	Convey("Given an OLS primitive", t, func() {
		op := NewFitOLS()

		Convey("recovers an exact line with zero residual", func() {
			design := [2][]float64{
				{1, 0, 1, 1, 1, 2, 1, 3, 1, 4},
				{1, 3, 5, 7, 9},
			}
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][2][]float64{design})))

			So(op.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			fit := out[0]
			So(fit[0], ShouldEqual, 1)
			So(fit[1], ShouldEqual, 2)
			So(fit[2], ShouldEqual, 5)
			So(fit[3], ShouldEqual, 2)
			So(fit[4], ShouldAlmostEqual, 0, 1e-9)
			So(fit[6], ShouldEqual, 1)
			So(len(fit), ShouldEqual, 11)
			So(fit[7], ShouldAlmostEqual, 1, 1e-9)
			So(fit[8], ShouldAlmostEqual, 2, 1e-9)
		})

		Convey("a rank-deficient design is undefined, not zero", func() {
			design := [2][]float64{
				{1, 2, 1, 2, 1, 2},
				{1, 2, 3},
			}
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][2][]float64{design})))

			So(op.Error(), ShouldBeNil)
			So(out[0][0], ShouldEqual, 0)
			So(math.IsNaN(out[0][5]), ShouldBeTrue)
			So(len(out[0]), ShouldEqual, 7)
		})

		Convey("n <= p is undefined", func() {
			design := [2][]float64{{1, 0, 1, 1}, {1, 2}}
			out := tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][2][]float64{design})))

			So(out[0][0], ShouldEqual, 0)
			So(out[0][2], ShouldEqual, 2)
			So(out[0][3], ShouldEqual, 2)
		})

		Convey("a ragged design records ErrShape", func() {
			fresh := NewFitOLS()
			design := [2][]float64{{1, 0, 1}, {1, 2}}
			out := tests.CollectSeq[[]float64](fresh.Next(tests.SliceToSeq([][2][]float64{design})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
