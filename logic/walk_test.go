package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestWalkTree(testingTB *testing.T) {
	Convey("Given a playbook branch with a failing condition", testingTB, func() {
		branches := []*Branch{{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:    SubjectHolding,
						Holding: &HoldingRef{Held: true},
					},
				}},
			},
			Action: &Action{Type: "noop"},
		}}

		trace := WalkTree(
			"BTC/EUR",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/EUR", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should record rejected steps without an action path", func() {
			So(trace.Symbol, ShouldEqual, "BTC/EUR")
			So(len(trace.Steps), ShouldEqual, 1)
			So(trace.Steps[0].Outcome, ShouldEqual, WalkOutcomeRejected)
			So(trace.ActivePath, ShouldBeNil)
		})
	})
}
