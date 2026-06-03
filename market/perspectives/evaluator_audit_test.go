package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBranchEvaluatorActionAudited(t *testing.T) {
	convey.Convey("Given a nested branch tree", t, func() {
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
		audit := &WalkAudit{}

		action := evaluator.ActionAudited(branches, audit)

		convey.Convey("It should record the branch trace and winning path", func() {
			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(action, convey.ShouldEqual, ActionIceberg)
			convey.So(len(audit.Steps), convey.ShouldEqual, 2)
			convey.So(audit.Steps[0].Pass, convey.ShouldBeTrue)
			convey.So(audit.Steps[0].Compared.Left, convey.ShouldEqual, 2)
			convey.So(audit.Steps[1].Pass, convey.ShouldBeTrue)
			convey.So(audit.Steps[1].Compared.Left, convey.ShouldEqual, 3)
			convey.So(audit.Steps[1].OnPath, convey.ShouldBeTrue)
			convey.So(audit.VerdictDepth, convey.ShouldEqual, 1)
		})
	})

	convey.Convey("Given a failing branch predicate", t, func() {
		evaluator := NewBranchEvaluator(BranchContext{
			Measurements: []Measurement{{Category: CategoryLaminar, SNR: 0.5}},
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
		audit := &WalkAudit{}

		action := evaluator.ActionAudited(branches, audit)

		convey.Convey("It should record the numeric failure", func() {
			convey.So(action, convey.ShouldBeNil)
			convey.So(len(audit.Steps), convey.ShouldEqual, 1)
			convey.So(audit.Steps[0].Pass, convey.ShouldBeFalse)
			convey.So(audit.Steps[0].FailReason, convey.ShouldEqual, "numeric")
			convey.So(audit.Steps[0].Compared.Pass, convey.ShouldBeFalse)
		})
	})
}

func BenchmarkBranchEvaluatorActionAudited(b *testing.B) {
	evaluator := NewBranchEvaluator(BranchContext{
		Measurements: []Measurement{{Category: CategoryLaminar, SNR: 2}},
		Observations: map[ObservationType]float64{ObservationNotHolding: 1},
	})
	branches := BranchList{{
		Category:    CategoryLaminar,
		Observation: ObservationNotHolding,
		Condition:   ConditionIsGreaterThanOrEqual,
		Unit:        UnitSNR,
		Value:       1,
		Action:      Action{Type: ActionLimit},
	}}
	audit := &WalkAudit{}

	for b.Loop() {
		audit.Steps = audit.Steps[:0]
		audit.SelectedPath = audit.SelectedPath[:0]
		audit.Verdict = nil
		_ = evaluator.ActionAudited(branches, audit)
	}
}
