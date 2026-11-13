package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFeatures(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the static Features table is inspected", func() {
			Convey("Labels match envelope signal metrics only", func() {
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
		})
	})
}

func TestForwardStoryNets(t *testing.T) {
	Convey("Setup", t, func() {
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

		Convey("When ForwardStoryNets folds the oracle curve", func() {
			enter, hold, exit, risk, ok := ForwardStoryNets(curve)

			Convey("Story nets match the folded weights", func() {
				So(ok, ShouldBeTrue)
				So(enter, ShouldEqual, 0.9+1.5)
				So(hold, ShouldEqual, 0.9+0.5)
				So(exit, ShouldEqual, 0.5+0.5-0.5)
				So(risk, ShouldEqual, 0.4)
			})
		})
	})
}
