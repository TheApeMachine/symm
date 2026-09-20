package algo

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestGaussJordan(t *testing.T) {
	Convey("Given GaussJordan solver", t, func() {
		solver := NewGaussJordan(func(any) float64 { return 1e-9 })

		Convey("Inverting a 2x2 identity matrix", func() {
			a := [][]float64{
				{1, 0},
				{0, 1},
			}
			b := [][]float64{
				{1, 0},
				{0, 1},
			}

			var sol [][]float64
			solver.SetDownstreamAny(func(ctx context.Context, out any) error { sol = out.([][]float64); return nil })
			solver.WriteAny(context.Background(), [2][][]float64{a, b})
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

			var sol [][]float64
			solver.SetDownstreamAny(func(ctx context.Context, out any) error { sol = out.([][]float64); return nil })
			solver.WriteAny(context.Background(), [2][][]float64{a, b})
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

			var sol [][]float64
			solver.SetDownstreamAny(func(ctx context.Context, out any) error { sol = out.([][]float64); return nil })
			solver.WriteAny(context.Background(), [2][][]float64{a, b})
			So(sol, ShouldBeNil)
		})
	})
}

func TestOLS(t *testing.T) {
	Convey("Given OLS solver", t, func() {
		ols := NewOLS(func(any) float64 { return 1e-9 })

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

			var beta []float64
			ols.SetDownstreamAny(func(ctx context.Context, out any) error { beta = out.([]float64); return nil })
			ols.WriteAny(context.Background(), [2][][]float64{x, y})
			So(beta, ShouldNotBeNil)
			So(len(beta), ShouldEqual, 2)
			So(beta[0], ShouldAlmostEqual, 1.0, 1e-9) // intercept = 1
			So(beta[1], ShouldAlmostEqual, 2.0, 1e-9) // slope = 2
		})
	})
}

func TestHayashiYoshida(t *testing.T) {
	Convey("Given Hayashi-Yoshida asynchronous covariance estimator", t, func() {
		hy := NewHayashiYoshida()

		Convey("Overlapping intervals accumulate returns", func() {
			var corr float64
			hy.SetDownstreamAny(func(ctx context.Context, out any) error { corr = out.(float64); return nil })
			hy.WriteAny(context.Background(), [2][2]int64{{0, 10}, {5, 15}})
			So(corr, ShouldBeGreaterThan, 0.0) // Just assert it compiles and calculates something
		})

		Convey("Non-overlapping intervals do not accumulate covariance", func() {
			var corr float64
			hy.SetDownstreamAny(func(ctx context.Context, out any) error { corr = out.(float64); return nil })
			hy.WriteAny(context.Background(), [2][2]int64{{0, 5}, {10, 15}})
			So(corr, ShouldBeLessThan, core.Unit)
		})
	})
}


