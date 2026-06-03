package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestFixturePlaybookBranches(t *testing.T) {
	convey.Convey("Given the stable test playbook", t, func() {
		branches := FixturePlaybookBranches()

		convey.Convey("It should be a tradable canonical registry", func() {
			canonical := CanonicalPlaybookBranches(branches)

			convey.So(HasTradablePlaybook(canonical), convey.ShouldBeTrue)
			convey.So(IsCanonicalPlaybook(canonical), convey.ShouldBeTrue)
		})

		convey.Convey("It should reach limit entry on fixture measurements", func() {
			rows := FixturePlaybookEntryMeasurements("BTC/EUR", 50_000)
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
}
