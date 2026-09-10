package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewCorrelation(t *testing.T) {
	Convey("Correlation is covariance over sqrt of the two energies", t, func() {
		op := equation.NewCorrelation[float64]()

		for _, covariance := range []float64{2, -2, 0, 1} {
			out := tests.CollectSeq(op.Next(transport.Values(equation.CorrelationInput[float64]{
				Covariance:  covariance,
				LeftEnergy:  1,
				RightEnergy: 2,
			})))
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, covariance/math.Sqrt2)
		}

		out := tests.CollectSeq(op.Next(transport.Values(equation.CorrelationInput[float64]{})))
		So(len(out), ShouldEqual, 1)
		So(math.IsNaN(out[0]), ShouldBeTrue)
	})
}
