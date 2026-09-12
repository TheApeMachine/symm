package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFisherEstimatorNext(t *testing.T) {
	Convey("Invalid correlations do not advance the estimator", t, func() {
		node := correlation.NewFisherEstimator()

		for _, value := range []float64{.2, .3, 1, -1, math.NaN(), .5} {
			out := tests.CollectSeq[correlation.FisherView](node.Next(transport.NewValues(value).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			valid := value > -1 && value < 1
			So(out[0].Defined, ShouldEqual, valid)

			if value == .5 {
				So(out[0].Baseline, ShouldAlmostEqual, math.Tanh((math.Atanh(.2)+math.Atanh(.3))/2))
				So(out[0].PriorCount, ShouldEqual, 2)
			}
		}
	})
}
