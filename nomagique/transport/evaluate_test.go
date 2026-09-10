package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
)

func TestEvaluate(t *testing.T) {
	Convey("Evaluate returns the single observation of a persisting fold", t, func() {
		add := arithmetic.NewAdd[float64, float64](0.0)

		for index, expected := range []float64{1, 3, 6, 10} {
			actual, err := Evaluate(add, Values(float64(index+1)))

			So(err, ShouldBeNil)
			So(actual, ShouldEqual, expected)
		}
	})
}

func TestEvaluateRejectsWrongShape(t *testing.T) {
	Convey("Evaluate rejects a missing operation and a run that is not one value", t, func() {
		_, err := Evaluate[float64, float64](nil, Values(0.0))
		So(err, ShouldNotBeNil)

		_, err = Evaluate(NewPass[float64](), Values(1.0, 2.0))
		So(err, ShouldNotBeNil)
	})
}
