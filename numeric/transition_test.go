package numeric

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTransitionMatrixSurprise(t *testing.T) {
	Convey("Given a transition matrix and padded observation", t, func() {
		matrix := NewTransitionMatrix(5, 0.1)
		observed := matrix.PadObserved([]float64{0.25, 0.25, 0.25, 0.25}, 1e-6)

		surprise, err := matrix.Surprise(observed)

		Convey("It should not return NaN", func() {
			So(err, ShouldBeNil)
			So(math.IsNaN(surprise), ShouldBeFalse)
		})
	})
}

func TestTransitionMatrixPadObserved(t *testing.T) {
	Convey("Given a four-class distribution", t, func() {
		matrix := NewTransitionMatrix(5, 0.1)
		padded := matrix.PadObserved([]float64{0.4, 0.3, 0.2, 0.1}, 1e-6)

		Convey("It should produce five normalized masses", func() {
			So(len(padded), ShouldEqual, 5)

			sum := 0.0

			for _, probability := range padded {
				sum += probability
			}

			So(sum, ShouldAlmostEqual, 1.0, 1e-9)
		})
	})
}

func TestTransitionMatrixUpdate(t *testing.T) {
	Convey("Given a transition update", t, func() {
		matrix := NewTransitionMatrix(5, 0.1)

		matrix.Update(2)

		Convey("It should advance last category", func() {
			So(matrix.lastCategory, ShouldEqual, 2)
		})
	})
}

func BenchmarkTransitionMatrixSurprise(b *testing.B) {
	matrix := NewTransitionMatrix(5, 0.1)
	observed := matrix.PadObserved([]float64{0.4, 0.3, 0.2, 0.1}, 1e-6)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = matrix.Surprise(observed)
	}
}
