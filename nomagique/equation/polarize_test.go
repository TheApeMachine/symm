package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPolarizeNext(t *testing.T) {
	Convey("Polarize splits a signed value against a scale of 10", t, func() {
		op := equation.NewPolarize[float64]()
		out := tests.CollectSeq(op.Next(transport.Values(
			equation.PolarizeInput[float64]{Value: 10, Scale: 10},
			equation.PolarizeInput[float64]{Value: -10, Scale: 10},
			equation.PolarizeInput[float64]{Value: 0, Scale: 10},
		)))

		So(out[0].Value, ShouldEqual, 0.5)
		So(out[1].Value, ShouldEqual, -0.5)
		So(out[2].Value, ShouldEqual, 0)
	})
}
