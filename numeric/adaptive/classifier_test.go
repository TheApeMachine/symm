package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClassifierNext(t *testing.T) {
	t.Parallel()

	Convey("Given a ternary move classifier", t, func() {
		classifier, err := NewClassifier(
			[]float64{-0.001, 0.001},
			[]float64{0, 1, 2},
			[]string{"dump", "precursor", "actual_pump"},
		)

		So(err, ShouldBeNil)

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
