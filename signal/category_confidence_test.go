package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategoryConfidence(t *testing.T) {
	Convey("Given decisive category scores", t, func() {
		confidence, err := CategoryConfidence([]float64{0, 0.8, 0.2, 0}, 2)

		Convey("It should preserve linear share when softmax dilutes", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldAlmostEqual, 0.8, 1e-9)
		})
	})

	Convey("Given a zero-based index request", t, func() {
		_, err := CategoryConfidence([]float64{0.8, 0.2}, 0)

		Convey("It should reject none indices for real categories", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestClassifierProbabilities(t *testing.T) {
	Convey("Given heterogeneous raw scores", t, func() {
		probabilities, err := ClassifierProbabilities([]float64{50, 1, 1})

		Convey("It should not collapse to a one-hot vector", func() {
			So(err, ShouldBeNil)
			So(probabilities[0], ShouldBeLessThan, 0.99)
		})
	})
}
