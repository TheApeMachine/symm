package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestEntryPassGatesFixture(t *testing.T) {
	convey.Convey("Given the stable fixture playbook entry root", t, func() {
		canonical := CanonicalPlaybookBranches(FixturePlaybookBranches())
		entryIndex := FindEntryIndex(canonical)

		convey.So(entryIndex, convey.ShouldEqual, 0)

		gates, found := entryPassGates(canonical[entryIndex])

		convey.Convey("It should list each entry gate category", func() {
			convey.So(found, convey.ShouldBeTrue)
			convey.So(len(gates), convey.ShouldEqual, 2)
			convey.So(gates[0].category, convey.ShouldEqual, CategorySystemicSlump)
			convey.So(gates[1].category, convey.ShouldEqual, CategoryVolumeStarvation)
		})
	})
}

func TestEntryPassMeasurements(t *testing.T) {
	convey.Convey("Given the stable fixture playbook", t, func() {
		branches := FixturePlaybookBranches()

		rows, rowsErr := EntryPassMeasurements("BTC/EUR", 50_000, branches)

		convey.Convey("It should derive rows that walk to an entry action", func() {
			convey.So(rowsErr, convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 2)

			evaluator := NewBranchEvaluator(BranchContext{
				Measurements: rows,
				Observations: map[ObservationType]float64{
					ObservationNotHolding: 1,
				},
			})
			action := evaluator.Action(CanonicalPlaybookBranches(branches))

			convey.So(evaluator.Err(), convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(IsEntryAction(*action), convey.ShouldBeTrue)
		})
	})

	convey.Convey("Given a minimal in-memory entry playbook", t, func() {
		branches := BranchList{{
			Category:    CategoryLaminar,
			Observation: ObservationNotHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       0,
			ValueSet:    true,
			Action:      Action{Type: ActionLimit},
		}}

		rows, rowsErr := EntryPassMeasurements("ETH/EUR", 3_000, branches)

		convey.Convey("It should emit one row per gate category", func() {
			convey.So(rowsErr, convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Category, convey.ShouldEqual, CategoryLaminar)
			convey.So(rows[0].Symbol, convey.ShouldEqual, "ETH/EUR")
		})
	})
}
