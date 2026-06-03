package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestSampleAdversarialMove(t *testing.T) {
	convey.Convey("Given theoretical and high-support moves", t, func() {
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
			nil,
		)

		moves := search.allMoves()
		branches := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}

		move := search.sampleAdversarialMove(moves, branches)

		convey.Convey("It should return a valid move from the candidate list", func() {
			convey.So(len(moves), convey.ShouldBeGreaterThan, 0)

			found := false

			for _, candidate := range moves {
				if candidate == move {
					found = true
					break
				}
			}

			convey.So(found, convey.ShouldBeTrue)
		})
	})
}

func TestMoveChainSupport(t *testing.T) {
	convey.Convey("Given a move without co-occurrence data", t, func() {
		search := &TreeSearch{}
		support := search.moveChainSupport(Move{}, perspectives.BranchList{})

		convey.Convey("It should report zero support", func() {
			convey.So(support, convey.ShouldEqual, 0)
		})
	})
}
