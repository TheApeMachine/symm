package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClassifierNext(t *testing.T) {
	t.Parallel()

	Convey("Given a ternary move classifier", t, func() {
		classifier := NewClassifier(
			[]float64{-0.001, 0.001},
			[]float64{0, 1, 2},
			[]string{"dump", "precursor", "actual_pump"},
		)

		Convey("It should map observations into classes", func() {
			dump, err := classifier.Next(0, -0.01)

			So(err, ShouldBeNil)
			So(dump, ShouldEqual, 0)
			So(classifier.Label(dump), ShouldEqual, "dump")

			precursor, err := classifier.Next(0, 0)

			So(err, ShouldBeNil)
			So(precursor, ShouldEqual, 1)

			pump, err := classifier.Next(0, 0.01)

			So(err, ShouldBeNil)
			So(pump, ShouldEqual, 2)
		})
	})
}

func TestClassifierConfidence(t *testing.T) {
	t.Parallel()

	Convey("Given the correlation herd classifier bands", t, func() {
		classifier := NewClassifier(
			[]float64{-0.30, 0.40, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"divergent_stress", "stochastic_noise", "decoupled_alpha", "systemic_herd"},
		)

		Convey("It should read high confidence deep inside a band", func() {
			So(classifier.Confidence(0.05), ShouldAlmostEqual, 1.0, 0.01)
		})

		Convey("It should read low confidence near a boundary", func() {
			So(classifier.Confidence(0.39), ShouldBeLessThan, 0.1)
		})

		Convey("It should stay category-agnostic for quiet noise", func() {
			So(classifier.Confidence(0.05), ShouldBeGreaterThan, classifier.Confidence(2.01))
		})
	})

	Convey("Given Code on the ternary classifier", t, func() {
		classifier := NewClassifier(
			[]float64{-0.001, 0.001},
			[]float64{0, 1, 2},
			[]string{"dump", "precursor", "actual_pump"},
		)

		Convey("It should match Next for the same observation", func() {
			code, err := classifier.Code(0.01)

			So(err, ShouldBeNil)
			So(code, ShouldEqual, 2)
		})
	})
}
