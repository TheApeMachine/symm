package budget

import (
	"runtime"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestApplyBudgetToTuneOptions(t *testing.T) {
	convey.Convey("Given unset tune options", t, func() {
		options := types.TuneOptions{}
		budget := types.SearchBudget{
			MeasurementSampleCap: 1000,
			MaxThresholds:        6,
			BeamWidth:            8,
			CandidateLimit:       64,
			MaxReasoningSteps:    3,
			HybridSeedCount:      4,
			MCTSIterations:       20,
			MinRoundTrips:        2,
			ComplexityPenalty:    0.1,
		}

		applyBudgetToTuneOptions(&options, budget)

		convey.Convey("It should fill zero-valued limits from the budget", func() {
			convey.So(options.Workers, convey.ShouldEqual, runtime.NumCPU())
			convey.So(options.MaxMeasurements, convey.ShouldEqual, 1000)
			convey.So(options.BeamWidth, convey.ShouldEqual, 8)
			convey.So(options.MCTSIterations, convey.ShouldEqual, 20)
			convey.So(options.Guard.ComplexityPenalty, convey.ShouldEqual, 0.1)
		})
	})
}

func TestApplyBudgetToScanOptions(t *testing.T) {
	convey.Convey("Given unset scan options", t, func() {
		budget := types.SearchBudget{
			MaxThresholds:     5,
			BeamWidth:         6,
			CandidateLimit:    30,
			MaxReasoningSteps: 2,
			MinRoundTrips:     1,
			ComplexityPenalty: 0.2,
		}

		options := applyBudgetToScanOptions(types.ScanOptions{}, budget)

		convey.Convey("It should copy budget limits into scan options", func() {
			convey.So(options.Workers, convey.ShouldEqual, runtime.NumCPU())
			convey.So(options.BeamWidth, convey.ShouldEqual, 6)
			convey.So(options.Budget, convey.ShouldResemble, budget)
		})
	})
}
