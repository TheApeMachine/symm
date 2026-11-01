package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeaturesMatchGridRegions(t *testing.T) {
	Convey("Features are one dim per RegionCount region", t, func() {
		So(len(Features), ShouldEqual, 23)
		So(Features[0].Label, ShouldEqual, "compression")
		So(Features[8].Label, ShouldEqual, "scarcity_score")
		So(Features[9].Source, ShouldEqual, "broker")
		So(Features[9].Label, ShouldEqual, "volume_veto")
		So(Features[10].Label, ShouldEqual, "volume_approve")
		So(Features[17].Label, ShouldEqual, "slump_score")
		So(Features[18].Label, ShouldEqual, "peak_score")
		So(Features[19].Label, ShouldEqual, "decoupled_score")
		So(Features[20].Label, ShouldEqual, "sync_score")
		So(Features[21].Label, ShouldEqual, "vacuum_score")
		So(Features[22].Label, ShouldEqual, "depth_score")
	})
}

func TestForwardStoryNets(t *testing.T) {
	Convey("ForwardStoryNets folds 23-dim action + oracle pairs", t, func() {
		curve := make([]float64, len(Features))
		for i := range curve {
			curve[i] = 0.5
		}
		curve[2] = 0.1
		curve[8] = 0.1
		for pair := range oraclePairs {
			curve[oracleFirst+2*pair] = 0.05
		}
		curve[9] = 0.4

		enter, hold, exit, risk, ok := ForwardStoryNets(curve)
		So(ok, ShouldBeTrue)
		// enter = 0.5+0.5-0.1 + 7*approve(0.5)
		So(enter, ShouldAlmostEqual, 0.9+3.5, 1e-9)
		// hold = 0.5+0.5-0.1 + board approve(0.5)
		So(hold, ShouldAlmostEqual, 0.9+0.5, 1e-9)
		So(exit, ShouldAlmostEqual, 0.5+0.5-0.5, 1e-9)
		So(risk, ShouldAlmostEqual, 0.4, 1e-9)
	})
}
