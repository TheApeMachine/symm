package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeatureNormalizer(t *testing.T) {
	Convey("Given a feature normalizer with exact prior moments", t, func() {
		normalizer := newFeatureNormalizer()

		Convey("Then the first two readings stay unscaled because no prior variance exists", func() {
			So(normalizer.Standardize(10), ShouldEqual, 0)
			So(normalizer.Standardize(12), ShouldEqual, 0)
		})

		Convey("Then later readings are standardized by the exact prior z-score", func() {
			normalizer.Standardize(10)
			normalizer.Standardize(12)

			standardized := normalizer.Standardize(14)

			So(standardized, ShouldAlmostEqual, 3/math.Sqrt(2))
		})

		Convey("Then a feature with zero prior variance remains centered at zero", func() {
			normalizer.Standardize(5)
			normalizer.Standardize(5)

			So(normalizer.Standardize(5), ShouldEqual, 0)
		})
	})
}
