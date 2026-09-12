package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestAdaptiveZScoreNext(t *testing.T) {
	Convey("Adaptive z-score uses log-space prior moments", t, func() {
		op := equation.NewAdaptiveZScore()
		results := tests.CollectSeq[equation.CausalResidualResult](op.Next(transport.NewValues(0.02, 0.01).Next(nil)))
		So(results[0].Baseline, ShouldAlmostEqual, 0.02, 1e-15)
		So(results[0].ZScore, ShouldEqual, 0)
		So(results[1].Baseline, ShouldAlmostEqual, 0.02, 1e-15)
		So(results[1].Residual, ShouldAlmostEqual, math.Log(0.5), 1e-15)
		So(results[1].ZScore, ShouldAlmostEqual, -1, 1e-12)
	})
}
