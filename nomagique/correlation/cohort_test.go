package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCohortNext(t *testing.T) {
	Convey("Admitted peers keep support-weighted summaries and reject the rest", t, func() {
		node := correlation.NewCohort()
		out := tests.CollectSeq[correlation.CohortSummary](node.Next(transport.NewValues(
			correlation.Peer{Score: .4, Support: 3, PeerEnergy: 2},
			correlation.Peer{Score: -.2, Support: 2, PeerEnergy: 4},
			correlation.Peer{Score: .9, Support: 1},
		).Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 1)
		mean := (3*.4 + 2*-.2) / 5.0
		So(out[0].PeersSeen, ShouldEqual, 3)
		So(out[0].Peers, ShouldEqual, 2)
		So(out[0].RejectedPeers, ShouldEqual, 1)
		So(out[0].TotalSupport, ShouldEqual, 5)
		So(out[0].EffectivePeers, ShouldAlmostEqual, 25.0/13)
		So(out[0].SignedScore, ShouldAlmostEqual, .16)
		So(out[0].AbsoluteScore, ShouldAlmostEqual, .32)
		So(out[0].PeerEnergyRate, ShouldAlmostEqual, 2.8)
		So(out[0].DispersionDefined, ShouldBeTrue)
		So(out[0].Dispersion, ShouldAlmostEqual, math.Sqrt((3*math.Pow(.4-mean, 2)+2*math.Pow(-.2-mean, 2))/5))

		Convey("Scores beyond one are summarized as they are, not through atanh", func() {
			wide := tests.CollectSeq[correlation.CohortSummary](correlation.NewCohort().Next(transport.NewValues(
				correlation.Peer{Score: 3, Support: 2},
				correlation.Peer{Score: -5, Support: 2},
			).Next(nil)))
			So(wide[0].SignedScore, ShouldAlmostEqual, -1)
			So(wide[0].Dispersion, ShouldAlmostEqual, 4)
		})

		Convey("One admitted peer has no dispersion", func() {
			single := tests.CollectSeq[correlation.CohortSummary](correlation.NewCohort().Next(transport.NewValues(
				correlation.Peer{Score: 2, Support: 4},
			).Next(nil)))
			So(single[0].Defined, ShouldBeTrue)
			So(single[0].DispersionDefined, ShouldBeFalse)
		})

		empty := tests.CollectSeq[correlation.CohortSummary](node.Next(transport.NewValues[correlation.Peer]().Next(nil)))
		So(node.Error(), ShouldBeNil)
		So(empty[0].Defined, ShouldBeFalse)
		So(empty[0].SignedScore, ShouldEqual, 0)
	})
}
