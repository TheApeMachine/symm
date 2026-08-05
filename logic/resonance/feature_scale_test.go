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

		Convey("Then a mature feature is standardized by the exact prior z-score", func() {
			readings := make([]float64, 0, featureWarmup)
			mean := 0.0

			for index := range featureWarmup {
				reading := float64(index)
				readings = append(readings, reading)
				mean += reading
				So(normalizer.Standardize(reading), ShouldEqual, 0)
			}

			mean /= float64(len(readings))
			variance := 0.0

			for _, reading := range readings {
				variance += math.Pow(reading-mean, 2)
			}

			variance /= float64(len(readings) - 1)
			reading := float64(featureWarmup)
			standardized := normalizer.Standardize(reading)

			So(standardized, ShouldAlmostEqual,
				(reading-mean)/math.Sqrt(variance))
		})

		Convey("Then a feature with zero prior variance remains centered at zero", func() {
			normalizer.Standardize(5)
			normalizer.Standardize(5)

			So(normalizer.Standardize(5), ShouldEqual, 0)
		})
	})
}
