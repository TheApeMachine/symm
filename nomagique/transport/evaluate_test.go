package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestEvaluate(t *testing.T) {
	Convey("Evaluate returns the single observation of a persisting fold", t, func() {
		sum := statistic.NewSum()

		for index, expected := range []float64{1, 3, 6, 10} {
			eval := transport.NewEvaluate(sum)
			var actual float64

			for out := range eval.Next(
				transport.NewValues(float64(index + 1)).Next(nil),
			) {
				actual = *(*float64)(out)
			}

			So(eval.Error(), ShouldBeNil)
			So(actual, ShouldEqual, expected)
		}
	})
}

func TestEvaluateRejectsWrongShape(t *testing.T) {
	Convey("Evaluate records a missing operation and a run that is not one value", t, func() {
		missing := transport.NewEvaluate(nil)
		tests.CollectSeq[float64](missing.Next(transport.NewValues(0.0).Next(nil)))
		So(missing.Error(), ShouldNotBeNil)

		multi := transport.NewEvaluate(transport.NewPass())
		tests.CollectSeq[float64](multi.Next(transport.NewValues(1.0, 2.0).Next(nil)))
		So(multi.Error(), ShouldNotBeNil)
	})
}
