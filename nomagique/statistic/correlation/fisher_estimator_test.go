package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestFisherEstimatorNext(t *testing.T) {
	Convey("Invalid correlations do not advance the estimator", t, func() {
		node := nmcorrelation.NewFisherEstimator()

		for _, value := range []float64{.2, .3, 1, -1, math.NaN(), .5} {
			out := tests.CollectSeq[nmcorrelation.FisherView](node.Next(sequence.NewValues(value).Next(nil)))
			So(node.Error(), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			valid := value > -1 && value < 1
			So(out[0].Defined, ShouldEqual, valid)

			if value == .5 {
				So(out[0].Baseline, ShouldAlmostEqual, math.Tanh((math.Atanh(.2)+math.Atanh(.3))/2))
				So(out[0].PriorCount, ShouldEqual, 2)

				baseline := tests.CollectSeq[float64](
					nmcorrelation.NewFisherBaseline().Next(sequence.NewValues(out[0]).Next(nil)),
				)
				So(baseline[0], ShouldAlmostEqual, out[0].Baseline)
			}
		}
	})
}
