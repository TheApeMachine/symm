package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeaturesMatchGridRegions(t *testing.T) {
	Convey("Features follow R00–R14 grid order", t, func() {
		So(len(Features), ShouldEqual, 15)
		So(Features[0].Label, ShouldEqual, "coiled_compression")
		So(Features[5].Label, ShouldEqual, "ask_ladder_release")
		So(Features[9].Label, ShouldEqual, "board_coiled")
		So(Features[14].Label, ShouldEqual, "capacity_stress")
	})
}

func TestForwardStoryNets(t *testing.T) {
	Convey("ForwardStoryNets mirrors gate net vectors", t, func() {
		curve := make([]float64, len(Features))
		for i := range curve {
			curve[i] = 0.5
		}
		curve[2] = 0.1  // starved opposition
		curve[8] = 0.1  // blowoff opposition
		curve[12] = 0.2
		curve[13] = 0.15
		curve[14] = 0.1

		enter, hold, exit, risk, ok := ForwardStoryNets(curve)
		So(ok, ShouldBeTrue)
		So(enter, ShouldAlmostEqual, 0.5+0.5-0.1+0.5, 1e-9)
		So(hold, ShouldAlmostEqual, 0.5+0.5-0.1+0.5, 1e-9)
		So(exit, ShouldAlmostEqual, 1.0, 1e-9)
		So(risk, ShouldAlmostEqual, 0.2, 1e-9)
	})
}
