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

		surprise := matrix.Surprise(observed)

		Convey("It should not return NaN", func() {
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

func TestTransitionMatrixNext(t *testing.T) {
	Convey("Given a transition dynamic call", t, func() {
		matrix := NewTransitionMatrix(5, 0.1)
		observed := matrix.PadObserved([]float64{0.5, 0.2, 0.2, 0.1}, 1e-6)

		surprise, err := matrix.Next(0, append([]float64{2}, observed...)...)

		Convey("It should score and advance state", func() {
			So(err, ShouldBeNil)
			So(math.IsNaN(surprise), ShouldBeFalse)
			So(matrix.lastCategory, ShouldEqual, 2)
		})
	})
}

func BenchmarkTransitionMatrixSurprise(b *testing.B) {
	matrix := NewTransitionMatrix(5, 0.1)
	observed := matrix.PadObserved([]float64{0.4, 0.3, 0.2, 0.1}, 1e-6)

	b.ReportAllocs()

	for b.Loop() {
		_ = matrix.Surprise(observed)
	}
}
