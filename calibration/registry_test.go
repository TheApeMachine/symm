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

func TestRegistryEdgeConfidenceRequiresSamples(t *testing.T) {
	Convey("Given insufficient calibration samples", t, func() {
		registry := NewRegistry()

		registry.Record(CalibrationTarget{
			Source:           logic.SourcePumpDump,
			Category:         logic.CategoryVerticalIgnition,
			PredictedMoveBps: 40,
			RealizedMoveBps:  20,
			CostBps:          10,
		}, 0.7)

		Convey("It should return zero edge confidence", func() {
			So(registry.EdgeConfidence(
				logic.SourcePumpDump,
				logic.CategoryVerticalIgnition,
				0.8,
			), ShouldEqual, 0)
		})
	})

	Convey("Given enough positive-edge samples", t, func() {
		registry := NewRegistry()

		for range MinCalibrationSamples {
			registry.Record(CalibrationTarget{
				Source:           logic.SourcePumpDump,
				Category:         logic.CategoryVerticalIgnition,
				PredictedMoveBps: 40,
				RealizedMoveBps:  30,
				CostBps:          10,
			}, 0.8)
		}

		confidence := registry.EdgeConfidence(
			logic.SourcePumpDump,
			logic.CategoryVerticalIgnition,
			0.8,
		)

		Convey("It should return conservative calibrated confidence", func() {
			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 0.8)
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

func TestSourceCategoryReports(t *testing.T) {
	Convey("Given populated calibration buckets", t, func() {
		registry := NewRegistry()

		registry.Record(CalibrationTarget{
			Source:           logic.SourceCVD,
			Category:         logic.CategoryAggressiveDrive,
			PredictedMoveBps: 18.4,
			RealizedMoveBps:  9.2,
			CostBps:          7.1,
		}, 0.7)

		reports := registry.SourceCategoryReports()

		Convey("It should render per-bucket diagnostics", func() {
			So(len(reports), ShouldEqual, 1)
			So(reports[0], ShouldContainSubstring, "source=cvd")
			So(reports[0], ShouldContainSubstring, "category=aggressive_drive")
		})
	})
}
