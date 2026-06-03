package playbook

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNestGateUnderEntry(t *testing.T) {
	convey.Convey("Given a flat entry and exit playbook", t, func() {
		entry := perspectives.Branch{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}
		exit := perspectives.Branch{
			Category:    perspectives.CategoryExhaustion,
			Observation: perspectives.ObservationHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
		}
		gate := perspectives.Branch{
			Category:    perspectives.CategoryRiskOnSurge,
			Observation: perspectives.ObservationNone,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
		}
		playbook := perspectives.BranchList{entry, exit}

		nested, ok := NestGateUnderEntry(playbook, gate)

		convey.Convey("It should nest the gate sequentially under the entry root", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(nested), convey.ShouldEqual, 2)
			convey.So(nested[0].Category, convey.ShouldEqual, perspectives.CategoryRiskOnSurge)
			convey.So(len(nested[0].Branches), convey.ShouldEqual, 1)
			convey.So(nested[0].Branches[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(ReasoningDepth(nested), convey.ShouldEqual, 2)
		})
	})
}

func TestWidenWithExit(t *testing.T) {
	convey.Convey("Given a playbook with an exit sibling", t, func() {
		playbook := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		alternateExit := perspectives.Branch{
			Category:    perspectives.CategoryActiveReversal,
			Observation: perspectives.ObservationHolding,
			Action:      perspectives.Action{Type: perspectives.ActionStopLossLimit},
		}

		widened, ok := widenWithExit(playbook, alternateExit)

		convey.Convey("It should swap the exit branch without changing depth", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(ReasoningDepth(widened), convey.ShouldEqual, ReasoningDepth(playbook))
			convey.So(widened[1].Category, convey.ShouldEqual, perspectives.CategoryActiveReversal)
		})
	})
}
