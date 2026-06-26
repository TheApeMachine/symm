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

func TestWalkTreeAction(testingTB *testing.T) {
	Convey("Given a branch that matches flat inventory", testingTB, func() {
		branches := []*Branch{{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:    SubjectHolding,
						Holding: &HoldingRef{Held: false},
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		action, trace := WalkTreeAction(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should return the first matching action", func() {
			So(action, ShouldNotBeNil)
			So(action.Type, ShouldEqual, ActionMarket)
			So(action.Symbol, ShouldEqual, "BTC/USD")
			So(len(trace.ActivePath), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a reusable branch action template", testingTB, func() {
		branches := []*Branch{{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:    SubjectHolding,
						Holding: &HoldingRef{Held: false},
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		first, _ := WalkTreeAction(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)
		second, _ := WalkTreeAction(
			"ETH/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "ETH/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should stamp each returned action without mutating the template", func() {
			So(first, ShouldNotBeNil)
			So(second, ShouldNotBeNil)
			So(first.Symbol, ShouldEqual, "BTC/USD")
			So(second.Symbol, ShouldEqual, "ETH/USD")
			So(branches[0].Action.Symbol, ShouldEqual, "")
		})
	})
}
