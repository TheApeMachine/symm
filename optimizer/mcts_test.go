package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNormalizeMCTSReward(t *testing.T) {
	convey.Convey("Given replay PnL scores", t, func() {
		search := &TreeSearch{rewardScale: 1}

		convey.Convey("It should map zero return to 0.5", func() {
			convey.So(search.normalizeMCTSReward(0), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})

		convey.Convey("It should map positive return above 0.5", func() {
			convey.So(search.normalizeMCTSReward(0.10), convey.ShouldBeGreaterThan, 0.5)
		})

		convey.Convey("It should map negative return below 0.5", func() {
			convey.So(search.normalizeMCTSReward(-0.10), convey.ShouldBeLessThan, 0.5)
		})
	})
}

func TestTreeSearchCachesMoves(t *testing.T) {
	convey.Convey("Given a hybrid tree search", t, func() {
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

		convey.Convey("It should reuse the pre-generated move list", func() {
			convey.So(len(search.cachedMoves), convey.ShouldBeGreaterThan, 0)
			convey.So(search.allMoves(), convey.ShouldResemble, search.cachedMoves)
		})
	})
}
