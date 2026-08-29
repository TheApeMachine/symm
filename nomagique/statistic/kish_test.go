package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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

		Convey("non-uniform weights yield the hand-calculated N_eff and maturity", func() {
			// w = [1, 0.5, 0.25]
			// sum_w  = 1.75
			// sum_w2 = 1 + 0.25 + 0.0625 = 1.3125
			// N_eff  = 1.75^2 / 1.3125 = 2.33333...
			// Maturity = 1 - 1/N_eff = 0.571428...
			weights := []float64{1, 0.5, 0.25}

			So(EffectiveSampleSize(weights), ShouldAlmostEqual, 2.3333333333333335, 1e-12)
			So(KishMaturity(weights), ShouldAlmostEqual, 0.5714285714285714, 1e-12)
		})
	})
}
