package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCohortNext(t *testing.T) {
	Convey("Admitted peers keep support-weighted summaries and reject the rest", t, func() {
		node := correlation.NewCohort(calculus.NewAtanh[float64]())
		out := tests.CollectSeq(node.Next(transport.Values(
			correlation.Peer{Correlation: .4, Support: 3, PeerEnergy: 2},
			correlation.Peer{Correlation: -.2, Support: 2, PeerEnergy: 4},
			correlation.Peer{Correlation: .9, Support: 1},
		)))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		z1, z2 := math.Atanh(.4), math.Atanh(-.2)
		mean := (3*z1 + 2*z2) / 5
		So(out[0].PeersSeen, ShouldEqual, 3)
		So(out[0].Peers, ShouldEqual, 2)
		So(out[0].RejectedPeers, ShouldEqual, 1)
		So(out[0].TotalSupport, ShouldEqual, 5)
		So(out[0].EffectivePeers, ShouldAlmostEqual, 25.0/13)
		So(out[0].SignedCorrelation, ShouldAlmostEqual, .16)
		So(out[0].AbsoluteCorrelation, ShouldAlmostEqual, .32)
		So(out[0].PeerEnergyRate, ShouldAlmostEqual, 2.8)
		So(out[0].Dispersion, ShouldAlmostEqual, math.Sqrt((3*z1*z1+2*z2*z2)/5-mean*mean))

		empty := tests.CollectSeq(node.Next(transport.Values[correlation.Peer]()))
		So(node.Error(), ShouldBeNil)
		So(empty[0].Defined, ShouldBeFalse)
		So(math.IsNaN(empty[0].SignedCorrelation), ShouldBeTrue)
	})
}
