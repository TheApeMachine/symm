package logic

import (
	"testing"
	"time"

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

func TestBranchSequentialStageFiresOnNextObservation(testingTB *testing.T) {
	Convey("Given a sequential parent and child on separate observations", testingTB, func() {
		branch := &Branch{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourcePumpDump,
						Category: NewCategory(CategoryCoiledCompression),
					},
				}},
			},
			Branches: []*Branch{{
				ConditionGroup: &ConditionGroup{
					Boolean: BooleanTypeAnd,
					Conditions: []Condition{{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourcePumpDump,
							Category: NewCategory(CategoryVerticalIgnition),
						},
					}},
				},
				Action: &Action{Type: ActionMarket, Side: SideBuy},
			}},
		}

		first := testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryCoiledCompression, 0.8, 1)
		first.SetTimestamp(time.Unix(0, 1).UnixNano())
		second := testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1)
		second.SetTimestamp(time.Unix(0, 2).UnixNano())

		initial, err := branch.Evaluate([]*datura.Artifact{first}, &Balances{})
		actions, err2 := branch.Evaluate([]*datura.Artifact{second}, &Balances{})

		Convey("It should park on the parent tick and emit on the next tick", func() {
			So(err, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(initial, ShouldBeNil)
			So(actions, ShouldHaveLength, 1)
			So(actions[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestWalkTreeActions(testingTB *testing.T) {
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

		actions, trace := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should return matching actions", func() {
			So(actions, ShouldHaveLength, 1)
			action := actions[0]
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

		firstActions, _ := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)
		secondActions, _ := WalkTreeActions(
			"ETH/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "ETH/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should stamp each returned action without mutating the template", func() {
			So(firstActions, ShouldHaveLength, 1)
			So(secondActions, ShouldHaveLength, 1)
			first := firstActions[0]
			second := secondActions[0]
			So(first, ShouldNotBeNil)
			So(second, ShouldNotBeNil)
			So(first.Symbol, ShouldEqual, "BTC/USD")
			So(second.Symbol, ShouldEqual, "ETH/USD")
			So(branches[0].Action.Symbol, ShouldEqual, "")
		})
	})
}
