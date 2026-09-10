package linear_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/equation/linear"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestLocalRegressionNext(t *testing.T) {
	Convey("Local regression is cumulative over elapsed seconds", t, func() {
		op := linear.NewLocalRegression()
		origin := int64(time.Second)
		out := tests.CollectSeq(op.Next(transport.Values(
			equation.Price{At: origin, Value: 0},
			equation.Price{At: origin + int64(time.Second), Value: 1},
			equation.Price{At: origin + 2*int64(time.Second), Value: 2},
			equation.Price{At: origin + 3*int64(time.Second), Value: 3},
		)))

		So(len(out), ShouldEqual, 4)
		So(out[0].SlopeDefined, ShouldBeFalse)
		So(out[3].SlopeDefined, ShouldBeTrue)
		So(out[3].Slope, ShouldEqual, 1)
	})
}
