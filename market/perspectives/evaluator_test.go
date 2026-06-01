package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBranchEvaluatorAction(t *testing.T) {
	convey.Convey("Given a leaf action branch", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{
				Category: CategoryLaminar,
				SNR:      2,
			}},
			Observations: map[ObservationType]float64{
				ObservationNotHolding: 1,
			},
		})
		branches := BranchList{{
			Category:    CategoryLaminar,
			Observation: ObservationNotHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			Action:      Action{Type: ActionLimit},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should return the branch action", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionLimit)
		})
	})

	convey.Convey("Given a deeper reachable action", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{
				Category:   CategoryLaminar,
				SNR:        2,
				Confidence: 3,
			}},
			Observations: map[ObservationType]float64{
				ObservationNotHolding: 1,
			},
		})
		branches := BranchList{{
			Category:    CategoryLaminar,
			Observation: ObservationNotHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			Action:      Action{Type: ActionMarket},
			Branches: []Branch{{
				Category:    CategoryLaminar,
				Observation: ObservationNotHolding,
				Condition:   ConditionIsGreaterThanOrEqual,
				Unit:        UnitConfidence,
				Value:       2,
				Action:      Action{Type: ActionIceberg},
			}},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should prefer the deepest action", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionIceberg)
		})
	})

	convey.Convey("Given an observation branch", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{Category: CategoryExhaustion, SNR: 2}},
			Observations: map[ObservationType]float64{
				ObservationHolding: 1,
			},
		})
		branches := BranchList{{
			Category:    CategoryExhaustion,
			Observation: ObservationHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			Action:      Action{Type: ActionSettlePosition},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should gate the action on holding state", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionSettlePosition)
		})
	})
}

func TestBranchEvaluatorActionUsesBranchers(t *testing.T) {
	convey.Convey("Given a branch metric from a measurement", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{
				Category: CategoryLaminar,
				Strength: 5,
			}},
			Observations: map[ObservationType]float64{
				ObservationNotHolding: 1,
			},
		})
		branches := BranchList{{
			Category:    CategoryLaminar,
			Observation: ObservationNotHolding,
			Metric:      "strength",
			Condition:   ConditionIsGreaterThan,
			Unit:        UnitPoints,
			Value:       4,
			Action:      Action{Type: ActionLimit},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should compare the named measurement field", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionLimit)
		})
	})

	convey.Convey("Given regime and metric branchers", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Regime: RegimeBullish,
			Observations: map[ObservationType]float64{
				ObservationHolding: 1,
			},
			Metrics: map[string]float64{
				"unrealized_return": 1.2,
			},
		})
		branches := BranchList{{
			Observation: ObservationHolding,
			Regime:      RegimeBullish,
			Metric:      "unrealized_return",
			Condition:   ConditionIsGreaterThan,
			Unit:        UnitPercentage,
			Value:       1,
			Action:      Action{Type: ActionTakeProfit},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should compare the metric in the requested unit", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionTakeProfit)
		})
	})
}

func TestBranchEvaluatorActionRejectsIncompatibleAction(t *testing.T) {
	convey.Convey("Given an exit action while not holding", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{Category: CategoryExhaustion, SNR: 2}},
			Observations: map[ObservationType]float64{
				ObservationNotHolding: 1,
			},
		})
		branches := BranchList{{
			Category:    CategoryExhaustion,
			Observation: ObservationNotHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			Action:      Action{Type: ActionTakeProfit},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should reject the action", func() {
			convey.So(action, convey.ShouldBeNil)
			convey.So(evaluator.Err(), convey.ShouldNotBeNil)
		})
	})

	convey.Convey("Given an action with no observation gate", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{Category: CategoryLaminar, SNR: 2}},
		})
		branches := BranchList{{
			Category:  CategoryLaminar,
			Condition: ConditionIsGreaterThanOrEqual,
			Unit:      UnitSNR,
			Value:     1,
			Action:    Action{Type: ActionMarket},
		}}

		action := evaluator.Action(branches)

		convey.Convey("It should reject the action", func() {
			convey.So(action, convey.ShouldBeNil)
			convey.So(evaluator.Err(), convey.ShouldNotBeNil)
		})
	})
}
