package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCohortNext(t *testing.T) {
	Convey("Admitted peers keep support-weighted summaries and reject the rest", t, func() {
		node := correlation.NewCohort()
		out := tests.CollectSeq[[11]float64](node.Next(tests.SliceToSeq([][3]float64{
			{.4, 3, 2},
			{-.2, 2, 4},
			{.9, 1, 0},
		})))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		z1, z2 := math.Atanh(.4), math.Atanh(-.2)
		mean := (3*z1 + 2*z2) / 5
		So(out[0][0], ShouldEqual, 3)
		So(out[0][1], ShouldEqual, 2)
		So(out[0][2], ShouldEqual, 1)
		So(out[0][3], ShouldEqual, 5)
		So(out[0][4], ShouldAlmostEqual, 25.0/13)
		So(out[0][5], ShouldAlmostEqual, .16)
		So(out[0][6], ShouldAlmostEqual, .32)
		So(out[0][7], ShouldAlmostEqual, 2.8)
		So(out[0][8], ShouldAlmostEqual, math.Sqrt((3*z1*z1+2*z2*z2)/5-mean*mean))

		empty := tests.CollectSeq[[11]float64](node.Next(tests.SliceToSeq([][3]float64{})))
		So(node.Error(), ShouldBeNil)
		So(empty[0][9], ShouldEqual, 0)
		So(empty[0][5], ShouldEqual, 0)
	})
}
