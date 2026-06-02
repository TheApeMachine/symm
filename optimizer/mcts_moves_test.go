package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestGenerateAllMoves(t *testing.T) {
	convey.Convey("Given a hybrid tree search profile", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		search := NewHybridTreeSearch(
			ctx,
			&profile,
			profile.Rows(),
			GuardOptions{},
			nil,
			MCTSOptions{Iterations: 1},
		)

		moves := search.generateAllMoves()

		convey.Convey("It should enumerate threshold and action moves", func() {
			convey.So(len(moves), convey.ShouldBeGreaterThan, 0)
			convey.So(search.allMoves(), convey.ShouldResemble, moves)
		})
	})
}

func TestApplyMove(t *testing.T) {
	convey.Convey("Given an existing branch and a threshold move", t, func() {
		ctx := context.Background()
		search := NewHybridTreeSearch(
			ctx,
			&Profile{ctx: ctx},
			nil,
			GuardOptions{},
			nil,
			MCTSOptions{Iterations: 1},
		)

		start := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}
		move := Move{
			category:    perspectives.CategoryLaminar,
			observation: perspectives.ObservationNotHolding,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       1,
		}

		branches := search.applyMove(start, move)

		convey.Convey("It should apply the move to the branch list", func() {
			convey.So(len(branches), convey.ShouldBeGreaterThan, 1)
			convey.So(branches[len(branches)-1].ValueSet, convey.ShouldBeTrue)
			convey.So(branches[len(branches)-1].Value, convey.ShouldEqual, 1)
		})
	})
}
