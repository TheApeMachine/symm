package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewPlannerConfig(t *testing.T) {
	Convey("Given the planner cold-start admission policy", t, func() {
		config := NewPlannerConfig()

		Convey("It should start at the no-information directional boundary", func() {
			So(config.MinimumConfidence, ShouldEqual, UninformativeDirectionConfidence)
			So(config.MinimumGraphScore, ShouldEqual, -1.0)
			So(config.MinimumUtility, ShouldEqual, 0.0)
		})
	})
}
