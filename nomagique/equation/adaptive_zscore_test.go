package equation_test

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestAdaptiveZScoreNext(t *testing.T) {
	Convey("AdaptiveZScore scores each arrival against its prior log-space baseline", t, func() {
		op := equation.NewAdaptiveZScore()
		out := tests.CollectSeq[[8]float64](op.Next(tests.SliceToSeq([]float64{1, math.E, 1})))

		So(op.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 3)
		So(out[0][0], ShouldEqual, 0)
		So(out[1][0], ShouldEqual, 1)
		So(out[1][1], ShouldAlmostEqual, 1)
		So(out[1][4], ShouldAlmostEqual, 1)
		So(out[2][1], ShouldAlmostEqual, math.Exp(0.5))
		So(out[2][6], ShouldBeLessThan, 0)
	})

	Convey("AdaptiveZScore records a domain error for a non-positive arrival", t, func() {
		op := equation.NewAdaptiveZScore()
		out := tests.CollectSeq[[8]float64](op.Next(tests.SliceToSeq([]float64{0})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
	})
}
