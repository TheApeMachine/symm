package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func TestPlannerSize(t *testing.T) {
	Convey("Given a decision that does not enter", t, func() {
		planner := &Planner{}
		decision := types.NewDecision(types.ActionNothing, "BTC/USD")

		Convey("It should leave the decision unchanged without requiring execution state", func() {
			sized, err := planner.size(decision)

			So(err, ShouldBeNil)
			So(sized, ShouldEqual, decision)
		})
	})

	Convey("Given an entry without an executable desk", t, func() {
		planner := &Planner{maxFraction: 0.20}
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")

		Convey("It should reject the incomplete allocation state", func() {
			_, err := planner.size(decision)

			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
		})
	})
}
