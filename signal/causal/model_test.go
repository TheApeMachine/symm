package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOlsStandardizesPredictorScales(t *testing.T) {
	Convey("Given predictors on very different scales", t, func() {
		target := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		macro := []float64{0.001, 0.002, 0.0015, 0.0018, 0.0022, 0.0011, 0.0019, 0.0021, 0.0016, 0.0017, 0.0020, 0.0014}
		liquidity := []float64{120, 118, 125, 130, 128, 122, 127, 129, 126, 124, 131, 123}

		coef, ok := ols(target, macro, liquidity)

		Convey("It should retain a non-zero macro coefficient", func() {
			So(ok, ShouldBeTrue)
			So(len(coef), ShouldEqual, 3)
			So(mathAbs(coef[1]), ShouldBeGreaterThan, 0)
		})
	})
}

func mathAbs(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
