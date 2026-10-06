package statistic

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestJointNext(t *testing.T) {
	Convey("Given a Joint primitive over two channels", t, func() {
		op := NewJoint(2)

		Convey("tracks per-channel residuals and the average energy", func() {
			vectors := [][]float64{
				{math.Log(10), math.Log(100)},
				{math.Log(20), math.Log(100)},
				{math.Log(40), math.Log(100)},
			}
			var readings [][]float64

			for _, vector := range vectors {
				for _, reading := range tests.CollectSeq[[]float64](op.Next(tests.SliceToSeq([][]float64{vector}))) {
					readings = append(readings, append([]float64(nil), reading...))
				}
			}

			So(op.Error(), ShouldBeNil)
			So(len(readings), ShouldEqual, 3)
			So(len(readings[0]), ShouldEqual, 18)

			first := readings[0]
			So(first[1], ShouldEqual, 0)
			So(first[2+0], ShouldEqual, 1)
			So(first[2+1], ShouldEqual, 0)

			last := readings[2]
			channel := last[2:10]
			So(channel[0], ShouldEqual, 3)
			So(channel[1], ShouldEqual, 1)
			So(channel[2], ShouldAlmostEqual, math.Sqrt(200), 1e-9)
			So(channel[3], ShouldAlmostEqual, math.Pow(math.Log(2), 2)/2, 1e-12)
			So(channel[7], ShouldAlmostEqual, channel[6]*channel[6], 1e-12)

			flat := last[10:18]
			So(math.IsNaN(flat[7]), ShouldBeTrue)
			So(last[1], ShouldEqual, 1)
			So(last[0], ShouldAlmostEqual, channel[7], 1e-12)
		})

		Convey("a mismatched dimension records ErrShape", func() {
			fresh := NewJoint(2)
			out := tests.CollectSeq[[]float64](fresh.Next(tests.SliceToSeq([][]float64{{1}})))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(fresh.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
