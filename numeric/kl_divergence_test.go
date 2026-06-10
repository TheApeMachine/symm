package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKLDivergence(t *testing.T) {
	Convey("Given aligned distributions", t, func() {
		observed := []float64{0.25, 0.25, 0.25, 0.25}
		expected := []float64{0.25, 0.25, 0.25, 0.25}
		sum := 1.0

		divergence, err := KLDivergence(observed, expected, sum, 1e-6)

		Convey("It should be near zero", func() {
			So(err, ShouldBeNil)
			So(divergence, ShouldAlmostEqual, 0, 1e-6)
		})
	})

	Convey("Given a zero expected sum", t, func() {
		observed := []float64{0.5, 0.5}
		expected := []float64{0, 0}

		divergence, err := KLDivergence(observed, expected, 0, 1e-6)

		Convey("It should not return NaN", func() {
			So(err, ShouldBeNil)
			So(math.IsNaN(divergence), ShouldBeFalse)
		})
	})

	Convey("Given a non-finite observation", t, func() {
		_, err := KLDivergence([]float64{math.NaN()}, []float64{0.5}, 1.0, 1e-6)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkKLDivergence(b *testing.B) {
	observed := []float64{1e-6, 0.3, 0.3, 0.2, 0.2}
	expected := []float64{0.2, 0.2, 0.2, 0.2, 0.2}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = KLDivergence(observed, expected, 1.0, 1e-6)
	}
}
