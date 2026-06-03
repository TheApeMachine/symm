package mcts

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestApplyBudget(t *testing.T) {
	convey.Convey("Given a derived search budget", t, func() {
		budget := types.SearchBudget{
			MCTSIterations:      64,
			MCTSSeedPriorVisits: 4,
			MaxReasoningSteps:   6,
			MaxThresholds:       8,
		}

		options := ApplyBudget(Options{}, budget)

		convey.Convey("It should fill unset MCTS limits from the budget", func() {
			convey.So(options.Iterations, convey.ShouldEqual, 64)
			convey.So(options.SeedPriorVisits, convey.ShouldEqual, 4)
			convey.So(options.MaxReasoningSteps, convey.ShouldEqual, 6)
			convey.So(options.MaxThresholds, convey.ShouldEqual, 8)
		})
	})
}
