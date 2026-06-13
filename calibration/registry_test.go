package calibration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestRegistryRecord(t *testing.T) {
	Convey("Given settled calibration targets", t, func() {
		registry := NewRegistry()

		registry.Record(CalibrationTarget{
			Source:           logic.SourcePumpDump,
			Category:         logic.CategoryVerticalIgnition,
			PredictedMoveBps: 40,
			RealizedMoveBps:  20,
			CostBps:          10,
		}, 0.7)

		meanEdge, ok := registry.MeanEdgeByBucket(
			logic.SourcePumpDump,
			logic.CategoryVerticalIgnition,
		)

		Convey("It should track realized edge by bucket", func() {
			So(ok, ShouldBeTrue)
			So(meanEdge, ShouldAlmostEqual, 10, 1e-9)
		})
	})
}
