package joint_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/joint"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestJointEstimatorNext(t *testing.T) {
	Convey("A joint estimator applies one Welford per coordinate", t, func() {
		op := joint.NewEstimator(equation.NewWelford(), equation.NewWelford())
		out := tests.CollectSeq(op.Next(transport.Values(
			joint.Input{Values: []float64{0.1, 0.2}},
			joint.Input{Values: []float64{0.2, 0.4}},
		)))
		So(op.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 2)
		So(len(out[1].Channels), ShouldEqual, 2)
		So(out[1].Channels[0].HasPrior, ShouldBeTrue)
	})
}
