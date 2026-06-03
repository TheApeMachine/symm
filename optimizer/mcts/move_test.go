package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestMove(t *testing.T) {
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

		Convey("It should expose search dimensions through accessors", func() {
			So(move.Category(), ShouldEqual, perspectives.CategoryLaminar)
			So(move.Observation(), ShouldEqual, perspectives.ObservationNotHolding)
		})
	})
}

func TestSearchEnumerations(t *testing.T) {
	Convey("Given MCTS search sets", t, func() {
		Convey("It should enumerate gate units and actions", func() {
			So(searchUnits, ShouldContain, perspectives.UnitSNR)
			So(searchExitActions, ShouldContain, perspectives.ActionSettlePosition)
		})
	})
}
