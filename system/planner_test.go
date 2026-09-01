package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewPlannerConfig(t *testing.T) {
	Convey("Given the live planner policy", t, func() {
		config := NewPlannerConfig()

		Convey("it should state allocation and cognition boundaries", func() {
			So(config.MaxAllocationFraction, ShouldBeGreaterThan, 0.0)
			So(config.CognitionSwitchConfidence, ShouldEqual, UninformativeDirectionConfidence)
		})
	})
}
