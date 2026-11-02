package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeaturesAreSignalMetrics(t *testing.T) {
	Convey("Features are real envelope signal labels only", t, func() {
		So(len(Features), ShouldEqual, 15)
		So(Features[0].Label, ShouldEqual, "compression")
		So(Features[8].Label, ShouldEqual, "scarcity_score")
		So(Features[9].Source, ShouldEqual, "sentiment")
		So(Features[9].Label, ShouldEqual, "slump_score")
		So(Features[14].Label, ShouldEqual, "depth_score")

		for _, feature := range Features {
			So(feature.Source, ShouldNotEqual, "broker")
		}
	})
}

func TestForwardStoryNets(t *testing.T) {
	Convey("ForwardStoryNets folds story + signal oracle pairs", t, func() {
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
		So(enter, ShouldAlmostEqual, 0.9+1.5, 1e-9)
		So(hold, ShouldAlmostEqual, 0.9+0.5, 1e-9)
		So(exit, ShouldAlmostEqual, 0.5+0.5-0.5, 1e-9)
		So(risk, ShouldAlmostEqual, 0.4, 1e-9)
	})
}
