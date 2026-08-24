package statistic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFitOLS(t *testing.T) {
	Convey("Given a linear system y = 1 + 2x", t, func() {
		design := make([]float64, 0, 100)
		targets := make([]float64, 0, 50)

		for index := 0; index < 50; index++ {
			x := float64(index) / 10
			design = append(design, 1, x)
			targets = append(targets, 1+2*x+0.001*float64(index%3-1))
		}

		fit := FitOLS(design, targets, 2)

		Convey("the fit is defined with the true coefficients", func() {
			So(fit.Defined, ShouldBeTrue)
			So(fit.Rank, ShouldEqual, 2)
			So(fit.Coefficients[1], ShouldAlmostEqual, 2, 0.01)
			So(fit.Coefficients[0], ShouldAlmostEqual, 1, 0.05)
		})

		Convey("duplicate columns are rank-deficient, not silently regularized", func() {
			duplicated := make([]float64, 0, 150)
			for index := 0; index < 50; index++ {
				x := float64(index) / 10
				duplicated = append(duplicated, 1, x, x)
			}

			deficient := FitOLS(duplicated, targets, 3)
			So(deficient.Defined, ShouldBeFalse)
			So(deficient.Rank, ShouldBeLessThan, 3)
		})

		Convey("insufficient rows are undefined", func() {
			small := FitOLS([]float64{1, 2}, []float64{1}, 2)
			So(small.Defined, ShouldBeFalse)
		})
	})
}

func TestCoefficientSNR(t *testing.T) {
	Convey("Given a defined coefficient and variance", t, func() {
		Convey("SNR is coefficient squared over variance", func() {
			So(CoefficientSNR(2, 1), ShouldEqual, 4)
		})

		Convey("undefined variance yields NaN, not zero", func() {
			So(math.IsNaN(CoefficientSNR(2, 0)), ShouldBeTrue)
			So(math.IsNaN(CoefficientSNR(2, math.NaN())), ShouldBeTrue)
		})

		Convey("zero coefficient yields a valid zero SNR", func() {
			So(CoefficientSNR(0, 1), ShouldEqual, 0)
		})
	})
}

func TestKishMaturity(t *testing.T) {
	Convey("Given equal weights", t, func() {
		Convey("effective sample size equals the count", func() {
			So(EffectiveSampleSize([]float64{1, 1, 1, 1}), ShouldEqual, 4)
		})

		Convey("maturity follows 1 - 1/N_eff", func() {
			So(KishMaturity([]float64{1, 1, 1, 1}), ShouldAlmostEqual, 0.75, 1e-12)
			So(KishMaturity([]float64{1}), ShouldEqual, 0)
			So(KishMaturity(nil), ShouldEqual, 0)
		})

		Convey("concentrated weights have lower effective support", func() {
			So(EffectiveSampleSize([]float64{1, 0, 0, 0}), ShouldEqual, 1)
		})
	})
}
