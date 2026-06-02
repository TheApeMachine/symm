package optimizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestMoveStruct(t *testing.T) {
	Convey("Given an MCTS move", t, func() {
		move := Move{
			depth:       2,
			category:    perspectives.CategoryLaminar,
			observation: perspectives.ObservationNotHolding,
			regime:      perspectives.RegimeTrending,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       1.5,
			action:      perspectives.ActionLimit,
		}

		Convey("It should retain search dimensions", func() {
			So(move.depth, ShouldEqual, 2)
			So(move.category, ShouldEqual, perspectives.CategoryLaminar)
			So(move.action, ShouldEqual, perspectives.ActionLimit)
		})
	})
}

func TestSearchEnumerations(t *testing.T) {
	Convey("Given optimizer search sets", t, func() {
		Convey("It should enumerate gate units and actions", func() {
			So(searchUnits, ShouldContain, perspectives.UnitSNR)
			So(searchExitActions, ShouldContain, perspectives.ActionSettlePosition)
		})
	})
}
