package statistic

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRegressionAccumulatorNext(t *testing.T) {
	Convey("Given a RegressionAccumulator primitive", t, func() {
		op := NewRegressionAccumulator(2)

		Convey("scores prequentially and matches the batch fit", func() {
			rows := [][]float64{
				{1, 0, 1.1},
				{1, 1, 2.9},
				{1, 2, 5.2},
				{1, 3, 6.8},
				{1, 4, 9.1},
			}
			var readings [][]float64

			for _, row := range rows {
				for _, reading := range tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][]float64{row}))) {
					readings = append(readings, append([]float64(nil), reading...))
				}
			}

			So(op.Error(), ShouldBeNil)
			So(len(readings), ShouldEqual, 5)
			So(readings[0][1], ShouldEqual, 0)
			So(readings[1][2], ShouldEqual, 0)
			So(readings[2][2], ShouldEqual, 1)
			So(readings[3][1], ShouldEqual, 1)

			last := readings[4]
			batch := tests.CollectSeq[[]float64](NewFitOLS().Next(tests.SliceToSeq([][2][]float64{{
				{1, 0, 1, 1, 1, 2, 1, 3, 1, 4},
				{1.1, 2.9, 5.2, 6.8, 9.1},
			}})))[0]

			So(last[3], ShouldEqual, 5)
			So(last[5], ShouldAlmostEqual, batch[4], 1e-9)
			So(last[8], ShouldAlmostEqual, batch[7], 1e-9)
			So(last[9], ShouldAlmostEqual, batch[8], 1e-9)
			So(last[7], ShouldEqual, 1)
			So(last[10], ShouldAlmostEqual, batch[9], 1e-9)
		})

		Convey("a wrong row length records ErrShape", func() {
			fresh := NewRegressionAccumulator(2)
			out := tests.CollectSeq[[]float64](fresh.Next(tests.SliceToSeq([][]float64{{1, 2}})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("a non-positive parameter count records ErrDomain", func() {
			fresh := NewRegressionAccumulator(0)

			So(errors.Is(fresh.Error(), core.ErrDomain), ShouldBeTrue)
		})
	})
}
