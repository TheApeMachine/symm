package algo

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGaussJordan(t *testing.T) {
	Convey("Given GaussJordan solver", t, func() {
		solver := NewGaussJordan(types.Const(1e-9))

		Convey("Inverting a 2x2 identity matrix", func() {
			a := [][]float64{
				{1, 0},
				{0, 1},
			}
			b := [][]float64{
				{1, 0},
				{0, 1},
			}

			sol := solver([2][][]float64{a, b})
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

			sol := solver([2][][]float64{a, b})
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

			sol := solver([2][][]float64{a, b})
			So(sol, ShouldBeNil)
		})
	})
}

func TestOLS(t *testing.T) {
	Convey("Given OLS solver", t, func() {
		ols := NewOLS(types.Const(1e-9))

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

			beta := ols([2][][]float64{x, y})
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
			stage1 := hy([2][2]int64{{0, 10}, {5, 15}})
			corr := stage1([2]float64{0.02, 0.02})
			So(corr, ShouldAlmostEqual, core.Unit, 1e-6)
		})

		Convey("Non-overlapping intervals do not accumulate covariance", func() {
			stage2 := hy([2][2]int64{{0, 5}, {10, 15}})
			corr := stage2([2]float64{0.01, 0.01})
			So(corr, ShouldBeLessThan, core.Unit)
		})
	})
}

func TestRLSAtoms(t *testing.T) {
	Convey("Given NewRLS atom closure", t, func() {
		rls := NewRLS(types.Const(2), types.Const(0.99))

		Convey("Steps adaptively on incoming feature vector and target", func() {
			res1 := rls([]float64{1.0, 2.0, 5.0})
			So(res1[0], ShouldEqual, 0.0) // initial prediction
			So(res1[1], ShouldBeGreaterThan, 0.0)

			res2 := rls([]float64{1.0, 2.0, 5.0})
			So(res2[0], ShouldBeGreaterThan, 0.0)
		})
	})
}
