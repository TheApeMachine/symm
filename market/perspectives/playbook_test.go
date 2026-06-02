package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestCanonicalPlaybookBranches(t *testing.T) {
	convey.Convey("Given flat deny gates with entry and exit siblings", t, func() {
		entry := Branch{
			Category:    CategoryLaminar,
			Observation: ObservationNotHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      Action{Type: ActionLimit},
		}
		exit := Branch{
			Category:    CategoryActiveReversal,
			Observation: ObservationHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      Action{Type: ActionSettlePosition},
		}
		deny := Branch{
			Category:    CategoryToxicBluff,
			Observation: ObservationNone,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			ValueSet:    true,
		}
		flat := BranchList{deny, entry, exit}

		canonical := CanonicalPlaybookBranches(flat)

		convey.Convey("It should nest deny gates under the entry root", func() {
			convey.So(len(canonical), convey.ShouldEqual, 2)
			convey.So(canonical[0].Category, convey.ShouldEqual, CategoryToxicBluff)
			convey.So(canonical[0].Branches[0].Category, convey.ShouldEqual, CategoryLaminar)
			convey.So(IsCanonicalPlaybook(canonical), convey.ShouldBeTrue)
			convey.So(HasTradablePlaybook(canonical), convey.ShouldBeTrue)
		})
	})
}

func TestIsCanonicalPlaybook(t *testing.T) {
	convey.Convey("Given top-level deny siblings beside entry and exit", t, func() {
		branches := BranchList{
			{Category: CategoryToxicBluff, Observation: ObservationNone, ValueSet: true},
			{
				Category:    CategoryLaminar,
				Observation: ObservationNotHolding,
				ValueSet:    true,
				Action:      Action{Type: ActionLimit},
			},
			{
				Category:    CategoryActiveReversal,
				Observation: ObservationHolding,
				ValueSet:    true,
				Action:      Action{Type: ActionSettlePosition},
			},
		}

		convey.Convey("It should not be canonical", func() {
			convey.So(IsCanonicalPlaybook(branches), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given deny gates nested under entry", t, func() {
		canonical := CanonicalPlaybookBranches(BranchList{
			{Category: CategoryToxicBluff, Observation: ObservationNone, ValueSet: true},
			{
				Category:    CategoryLaminar,
				Observation: ObservationNotHolding,
				ValueSet:    true,
				Action:      Action{Type: ActionLimit},
			},
			{
				Category:    CategoryActiveReversal,
				Observation: ObservationHolding,
				ValueSet:    true,
				Action:      Action{Type: ActionSettlePosition},
			},
		})

		convey.Convey("It should be canonical", func() {
			convey.So(IsCanonicalPlaybook(canonical), convey.ShouldBeTrue)
		})
	})
}
