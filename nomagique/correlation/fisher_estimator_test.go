package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFisherEstimatorNext(t *testing.T) {
	Convey("Invalid correlations do not advance the estimator", t, func() {
		node := correlation.NewFisherEstimator()

		for _, value := range []float64{.2, .3, 1, -1, math.NaN(), .5} {
			out := tests.CollectSeq[[10]float64](node.Next(tests.SliceToSeq([]float64{value})))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			valid := 0.0

			if value > -1 && value < 1 {
				valid = 1
			}

			So(out[0][1], ShouldEqual, valid)

			if value == .5 {
				So(out[0][2], ShouldAlmostEqual, math.Tanh((math.Atanh(.2)+math.Atanh(.3))/2))
				So(out[0][4], ShouldEqual, 2)
			}
		}
	})
}
