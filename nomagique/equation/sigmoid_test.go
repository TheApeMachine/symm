package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewSigmoid(t *testing.T) {
	Convey("Sigmoid matches 1/(1+exp(-x))", t, func() {
		op := equation.NewSigmoid[float64]()

		for _, value := range []float64{0, 10, -10} {
			out := tests.CollectSeq(op.Next(transport.Values(value)))
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldEqual, 1/(1+math.Exp(-value)))
		}
	})
}
