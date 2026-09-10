package hawkes_test

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation/hawkes"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestBranchingNext(t *testing.T) {
	Convey("Stable branching ratios recover stationary means", t, func() {
		node := hawkes.NewBranching()
		rng := rand.New(rand.NewSource(734))

		for range 30 {
			a, b, c, d := 0.25*rng.Float64(), 0.25*rng.Float64(), 0.25*rng.Float64(), 0.25*rng.Float64()
			beta, muX, muY := 0.5+rng.Float64(), 0.2+rng.Float64(), 0.2+rng.Float64()
			out := tests.CollectSeq(node.Next(transport.Values(hawkes.Parameters{
				MuX: muX, MuY: muY,
				AlphaXX: a * beta, AlphaXY: b * beta, AlphaYX: c * beta, AlphaYY: d * beta,
				Beta: beta,
			})))
			So(node.Error(), ShouldBeNil)
			f := out[0]
			determinant := (1-a)*(1-d) - b*c
			So(f.SpectralRadius, ShouldAlmostEqual, (a+d+math.Sqrt((a-d)*(a-d)+4*b*c))/2)
			So(f.MeanX, ShouldAlmostEqual, ((1-d)*muX+b*muY)/determinant)
			So(f.MeanY, ShouldAlmostEqual, (c*muX+(1-a)*muY)/determinant)
			So(f.DescendantsX, ShouldAlmostEqual, (1-d+c)/determinant-1)
			So(f.DescendantsY, ShouldAlmostEqual, (1-a+b)/determinant-1)
			So(f.Defined, ShouldBeTrue)
		}
	})

	Convey("A critical system does not invent a stationary mean", t, func() {
		node := hawkes.NewBranching()
		out := tests.CollectSeq(node.Next(transport.Values(hawkes.Parameters{
			MuX: 1, MuY: 1, AlphaXX: 1, AlphaXY: 0, AlphaYX: 0, AlphaYY: 0.5, Beta: 1,
		})))
		So(out[0].Defined, ShouldBeFalse)
		So(math.IsNaN(out[0].MeanX), ShouldBeTrue)
	})
}
