package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/mcts"
)

func TestDecisionWire(t *testing.T) {
	Convey("Given a decision carrying a recursive MCTS trace", t, func() {
		decision := *NewDecision(ActionNothing, "BTC/USD")
		decision.Trace = &DecisionTrace{
			MCTS: DecisionMCTSTrace{
				Branches: []DecisionMCTSBranch{
					{Action: "first"},
					{Action: "second"},
				},
				Tree: &mcts.SearchNode{},
			},
		}

		Convey("It should encode only the branches and tree requested by the consumer", func() {
			focused := DecisionWire(decision, 2, true)
			summary := DecisionWire(decision, 1, false)

			So(focused.Trace, ShouldNotBeNil)
			So(focused.Trace.Branches, ShouldHaveLength, 2)
			So(focused.Trace.Tree, ShouldNotBeNil)
			So(summary.Trace, ShouldNotBeNil)
			So(summary.Trace.Branches, ShouldHaveLength, 1)
			So(summary.Trace.Tree, ShouldBeNil)
		})
	})
}

func BenchmarkDecisionWire(b *testing.B) {
	decision := *NewDecision(ActionNothing, "BTC/USD")
	decision.Trace = &DecisionTrace{
		MCTS: DecisionMCTSTrace{
			Tree: &mcts.SearchNode{},
		},
	}
	b.ReportAllocs()

	for b.Loop() {
		DecisionWire(decision, 2, false)
	}
}
