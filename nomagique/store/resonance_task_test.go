package store

import (
	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
	"math"
	"testing"
)

func TestResonanceTaskEvaluate(t *testing.T) {
	Convey("The retained affine head matches least squares without prior observations", t, func() {
		task := newResonanceTask(2)
		empty := task.Evaluate([]float64{2, 3}, 0, false)
		So(empty.Ready, ShouldBeFalse)
		So(task.observations, ShouldEqual, 0)
		design := mat.NewDense(64, 3, nil)
		target := mat.NewDense(64, 1, nil)
		for index := range 64 {
			first, second := math.Sin(float64(index)), math.Cos(float64(index)/3)
			value := 2 + 3*first - second + math.Sin(float64(index)/7)/10
			design.Set(index, 0, 1)
			design.Set(index, 1, first)
			design.Set(index, 2, second)
			target.Set(index, 0, value)
			task.Evaluate([]float64{first, second}, value, true)
		}
		var expected mat.Dense
		So(expected.Solve(design, target), ShouldBeNil)
		for coordinate := range 3 {
			So(task.beta[coordinate], ShouldAlmostEqual, expected.At(coordinate, 0), 1e-8)
		}
		So(task.rank, ShouldEqual, 3)
		So(task.observations, ShouldEqual, 64)
		forecast := task.Evaluate([]float64{.5, .75}, 0, false)
		So(forecast.Ready, ShouldBeTrue)
		So(forecast.DegreesOfFreedom, ShouldEqual, 61)
		So(forecast.Prediction, ShouldAlmostEqual, expected.At(0, 0)+.5*expected.At(1, 0)+.75*expected.At(2, 0), 1e-8)
		Convey("Repeated collinear observations retain the actual rank", func() {
			singular := newResonanceTask(2)
			for value := range 64 {
				singular.Evaluate([]float64{float64(value), 2 * float64(value)}, 3*float64(value), true)
			}
			So(singular.rank, ShouldEqual, 2)
			So(singular.Evaluate([]float64{7, 14}, 0, false).Prediction, ShouldAlmostEqual, 21, 1e-8)
		})
	})
}
