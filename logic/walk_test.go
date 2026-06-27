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
			So(actions[0].EntryConfidence, ShouldEqual, 0.8)
			So(actions[0].ReasonSource, ShouldEqual, SourcePumpDump)
			So(actions[0].ReasonCategory, ShouldEqual, CategoryVerticalIgnition)
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
				}, {
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourceFluid,
						Category: NewCategory(CategoryLaminar),
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		actions, trace, err := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should return matching actions", func() {
			So(err, ShouldBeNil)
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
				}, {
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourceFluid,
						Category: NewCategory(CategoryLaminar),
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		firstActions, _, firstErr := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "BTC/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)
		secondActions, _, secondErr := WalkTreeActions(
			"ETH/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourceFluid, "ETH/USD", CategoryLaminar, 0.5, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should stamp each returned action without mutating the template", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
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

func TestWalkTreeActionsStampsWeakestPositiveEvidenceConfidence(testingTB *testing.T) {
	Convey("Given a branch matched by two positive signal conditions", testingTB, func() {
		branches := []*Branch{{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{
					{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourcePumpDump,
							Category: NewCategory(CategoryOrganicTrend),
						},
					},
					{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourceSentiment,
							Category: NewCategory(CategoryRiskOnSurge),
						},
					},
				},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		actions, _, err := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryOrganicTrend, 0.9, 1.0),
				testMeasurementArtifact(SourceSentiment, "BTC/USD", CategoryRiskOnSurge, 0.7, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should price the action by the weakest required evidence", func() {
			So(err, ShouldBeNil)
			So(actions, ShouldHaveLength, 1)
			So(actions[0].EntryConfidence, ShouldEqual, 0.7)
			So(actions[0].ReasonSource, ShouldEqual, SourceSentiment)
			So(actions[0].ReasonCategory, ShouldEqual, CategoryRiskOnSurge)
		})
	})
}

func TestWalkTreeActionsSupportsNestedConfirmationGroups(testingTB *testing.T) {
	Convey("Given a branch that requires A and either B or C", testingTB, func() {
		branch := &Branch{
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
				Groups: []ConditionGroup{{
					Boolean: BooleanTypeOr,
					Conditions: []Condition{{
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourceHawkes,
							Category: NewCategory(CategoryFrenzy),
						},
					}, {
						Type: ConditionIsTrue,
						Left: ConditionOperand{
							Type:     SubjectCategory,
							Source:   SourceCVD,
							Category: NewCategory(CategoryAggressiveDrive),
						},
					}},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}

		withoutConfirmation, _, missingErr := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1.0),
			},
			&Balances{},
			[]*Branch{branch},
		)

		withConfirmation, _, confirmedErr := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1.0),
				testMeasurementArtifact(SourceHawkes, "BTC/USD", CategoryFrenzy, 0.7, 1.0),
			},
			&Balances{},
			[]*Branch{branch},
		)

		Convey("It should not propose until one confirming branch is present", func() {
			So(missingErr, ShouldBeNil)
			So(withoutConfirmation, ShouldBeNil)
			So(confirmedErr, ShouldBeNil)
			So(withConfirmation, ShouldHaveLength, 1)
			So(withConfirmation[0].EntryConfidence, ShouldEqual, 0.7)
			So(withConfirmation[0].ReasonSource, ShouldEqual, SourceHawkes)
			So(withConfirmation[0].ReasonCategory, ShouldEqual, CategoryFrenzy)
		})
	})
}

func TestFalseGuardRequiresMeasurementEvidence(testingTB *testing.T) {
	Convey("Given an entry guarded by a missing toxicity measurement", testingTB, func() {
		branches := []*Branch{{
			ConditionGroup: &ConditionGroup{
				Boolean: BooleanTypeAnd,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourcePumpDump,
						Category: NewCategory(CategoryVerticalIgnition),
					},
				}, {
					Type: ConditionIsFalse,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourceToxicity,
						Category: NewCategory(CategoryToxicBluff),
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}}

		actions, _, err := WalkTreeActions(
			"BTC/USD",
			[]*datura.Artifact{
				testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryVerticalIgnition, 0.8, 1.0),
			},
			&Balances{},
			branches,
		)

		Convey("It should not treat absence as a passed safety guard", func() {
			So(err, ShouldBeNil)
			So(actions, ShouldBeNil)
		})
	})
}

func TestBranchRequiresMinimumObservations(testingTB *testing.T) {
	Convey("Given a branch that requires two distinct matching observations", testingTB, func() {
		branch := &Branch{
			ConditionGroup: &ConditionGroup{
				Boolean:         BooleanTypeAnd,
				MinObservations: 2,
				Conditions: []Condition{{
					Type: ConditionIsTrue,
					Left: ConditionOperand{
						Type:     SubjectCategory,
						Source:   SourceCVD,
						Category: NewCategory(CategoryAggressiveDrive),
					},
				}},
			},
			Action: &Action{Type: ActionMarket, Side: SideBuy, Fraction: 0.2},
		}

		first := testMeasurementArtifact(SourceCVD, "BTC/USD", CategoryAggressiveDrive, 0.8, 1.0)
		first.SetTimestamp(time.Unix(0, 1).UnixNano())
		second := testMeasurementArtifact(SourceCVD, "BTC/USD", CategoryAggressiveDrive, 0.7, 1.0)
		second.SetTimestamp(time.Unix(0, 2).UnixNano())

		firstActions, firstErr := branch.Evaluate([]*datura.Artifact{first}, &Balances{})
		secondActions, secondErr := branch.Evaluate([]*datura.Artifact{second}, &Balances{})

		Convey("It should park on the first observation and propose on the second", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(firstActions, ShouldBeNil)
			So(secondActions, ShouldHaveLength, 1)
			So(secondActions[0].EntryConfidence, ShouldEqual, 0.7)
		})
	})
}
