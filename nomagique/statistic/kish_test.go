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
	})
}
