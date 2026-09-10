package hawkes_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation/hawkes"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLikelihoodNext(t *testing.T) {
	Convey("Likelihood scores a short bivariate path", t, func() {
		op := hawkes.NewLikelihood()
		out := tests.CollectSeq(op.Next(transport.Values(hawkes.LikelihoodInput{
			Parameters: hawkes.Parameters{
				MuX: 0.5, MuY: 0.5, AlphaXX: 0.1, AlphaXY: 0.05, AlphaYX: 0.05, AlphaYY: 0.1, Beta: 1,
			},
			Origin: 0, Horizon: 2,
			Events: []hawkes.Event{{Side: 0, At: 0.5}, {Side: 1, At: 1.5}},
		})))
		So(op.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		So(out[0].LogLikelihood < 0, ShouldBeTrue)
	})
}
