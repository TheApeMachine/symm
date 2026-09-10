package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCausalResidualNext(t *testing.T) {
	Convey("Causal residual scores against prior moments", t, func() {
		welford := equation.NewWelford()
		residual := equation.NewCausalResidual()
		out := tests.CollectSeq(residual.Next(welford.Next(transport.Values(1.0, 3.0, 5.0))))
		So(len(out), ShouldEqual, 3)
		So(out[0].HasPrior, ShouldBeFalse)
		So(out[1].HasPrior, ShouldBeTrue)
		So(out[1].Baseline, ShouldEqual, 1)
		So(out[1].Residual, ShouldEqual, 2)
	})
}
