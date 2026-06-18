package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey")

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
			[]Measurement{{Source: SourceFluid, Symbol: "BTC/EUR", Confidence: 0.5}},
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
