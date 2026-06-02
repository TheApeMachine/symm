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

		Convey("It should read higher standout deep inside a band than on a boundary", func() {
			So(classifier.Standout(0.05), ShouldBeGreaterThan, classifier.Standout(0.39))
		})
	})

	Convey("Given the pumpdump ignition classifier bands", t, func() {
		classifier := NewClassifier(
			[]float64{-0.10, 0.50, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
		)

		Convey("It should keep clarity and standout on the unit interval at pathological inputs", func() {
			clarity := classifier.Confidence(131_996_665_001.92592)
			standout := classifier.Standout(131_996_665_001.92592)

			So(clarity, ShouldBeGreaterThan, 0)
			So(clarity, ShouldBeLessThanOrEqualTo, 1)
			So(standout, ShouldBeGreaterThanOrEqualTo, 0)
			So(standout, ShouldBeLessThanOrEqualTo, 1)
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
