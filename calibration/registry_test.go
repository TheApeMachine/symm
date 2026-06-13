package calibration

import (
	"sync"
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

func TestRegistryRecordConcurrent(t *testing.T) {
	Convey("Given concurrent calibration records", t, func() {
		registry := NewRegistry()
		waitGroup := sync.WaitGroup{}

		for range 32 {
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()

				registry.Record(CalibrationTarget{
					Source:           logic.SourcePumpDump,
					Category:         logic.CategoryVerticalIgnition,
					PredictedMoveBps: 40,
					RealizedMoveBps:  20,
					CostBps:          10,
				}, 0.7)
			}()
		}

		waitGroup.Wait()

		meanEdge, ok := registry.MeanEdgeByBucket(
			logic.SourcePumpDump,
			logic.CategoryVerticalIgnition,
		)

		Convey("It should accumulate all records without loss", func() {
			So(ok, ShouldBeTrue)
			So(meanEdge, ShouldAlmostEqual, 10, 1e-9)
			So(registry.state.Load().buckets[bucketKey(
				logic.SourcePumpDump,
				logic.CategoryVerticalIgnition,
			)].count, ShouldEqual, 32)
		})
	})
}
