package system_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestLearning_ApplyResonance(t *testing.T) {
	Convey("Given learning defaults", t, func() {
		learning := system.NewLearning()

		Convey("It patches resonance gate floors and bypass", func() {
			learning.ApplyResonance(map[string]any{
				"type":                        "resonance",
				"bypass":                      true,
				"min_task_relative_precision": 0.1,
				"min_surprise":                0.2,
				"min_supported_horizon":       1.0,
				"min_enter_z":                 0.0,
			})

			So(learning.ResonanceBypass, ShouldBeTrue)
			So(learning.MinTaskRelativePrecision, ShouldEqual, 0.1)
			So(learning.MinSurprise, ShouldEqual, 0.2)
			So(learning.MinSupportedHorizon, ShouldEqual, 1)
			So(learning.MinEnterZ, ShouldEqual, 0.0)
		})
	})
}
