package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFisherEstimatorNext(t *testing.T) {
	Convey("Invalid correlations do not advance the estimator", t, func() {
		moments := equation.NewWelford()
		node := correlation.NewFisherEstimator(moments)

		for _, value := range []float64{.2, .3, 1, -1, math.NaN(), .5} {
			out, err := transport.Evaluate(node, transport.Values(value))
			So(err, ShouldBeNil)
			valid := value > -1 && value < 1
			So(out.Defined, ShouldEqual, valid)

			if !valid {
				So(moments.Read().Count, ShouldEqual, 2)
			}

			if value == .5 {
				So(out.Baseline, ShouldAlmostEqual, math.Tanh((math.Atanh(.2)+math.Atanh(.3))/2))
				So(out.PriorCount, ShouldEqual, 2)
			}
		}
	})
}
