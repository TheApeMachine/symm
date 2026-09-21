package algo

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGaussJordan(t *testing.T) {
	Convey("Given GaussJordan solver", t, func() {
		solver := NewGaussJordan()

		Convey("Inverting a 2x2 identity matrix", func() {
			a := [][]float64{
				{1, 0},
				{0, 1},
			}
			b := [][]float64{
				{1, 0},
				{0, 1},
			}

			sol, err := solver.Evaluate(context.Background(), a, b)
			So(err, ShouldBeNil)
			So(sol, ShouldNotBeNil)
			So(sol[0][0], ShouldAlmostEqual, 1.0, 1e-9)
			So(sol[0][1], ShouldAlmostEqual, 0.0, 1e-9)
			So(sol[1][0], ShouldAlmostEqual, 0.0, 1e-9)
			So(sol[1][1], ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("Solving 2x2 linear system 2x + y = 5, x + 3y = 5", func() {
			a := [][]float64{
				{2, 1},
				{1, 3},
			}
			b := [][]float64{
				{5},
				{5},
			}

			sol, err := solver.Evaluate(context.Background(), a, b)
			So(err, ShouldBeNil)
			So(sol, ShouldNotBeNil)
			So(sol[0][0], ShouldAlmostEqual, 2.0, 1e-9) // x = 2
			So(sol[1][0], ShouldAlmostEqual, 1.0, 1e-9) // y = 1
		})

		Convey("Singular system returns nil", func() {
			a := [][]float64{
				{1, 2},
				{2, 4},
			}
			b := [][]float64{
				{1},
				{2},
			}

			sol, err := solver.Evaluate(context.Background(), a, b)
			So(err, ShouldBeNil)
			So(sol, ShouldBeNil)
		})
	})
}

func TestOLS(t *testing.T) {
	Convey("Given OLS solver", t, func() {
		ols := NewOLS()

		Convey("Estimating y = 2x + 1", func() {
			x := [][]float64{
				{1, 1},
				{1, 2},
				{1, 3},
				{1, 4},
				{1, 5},
			}
			y := [][]float64{
				{3},
				{5},
				{7},
				{9},
				{11},
			}

			beta, err := ols.Evaluate(context.Background(), x, y)
			So(err, ShouldBeNil)
			So(beta, ShouldNotBeNil)
			So(len(beta), ShouldEqual, 2)
			So(beta[0], ShouldAlmostEqual, 1.0, 1e-9) // intercept = 1
			So(beta[1], ShouldAlmostEqual, 2.0, 1e-9) // slope = 2
		})
	})
}
