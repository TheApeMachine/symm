package matrix_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDeterminant2Next(t *testing.T) {
	Convey("Determinant2 owns ad - bc over row-major [4]float64", t, func() {
		node := matrix.NewDeterminant2()
		out := tests.CollectSeq[float64](node.Next(data.NewValue([4]float64{1, 2, 3, 4}, [4]float64{2, 0, 0, 3})))
		So(node.Error(), ShouldBeNil)
		So(out, ShouldResemble, []float64{-2, 6})
	})
}

func TestSpectralRadius2Next(t *testing.T) {
	Convey("SpectralRadius2 returns the largest eigenvalue magnitude", t, func() {
		node := matrix.NewSpectralRadius2()
		out := tests.CollectSeq[float64](node.Next(data.NewValue(
			[4]float64{2, 0, 0, -3},
			[4]float64{0, -1, 1, 0},
		)))
		So(node.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 3)
		So(out[1], ShouldAlmostEqual, math.Sqrt(1))
	})
}
