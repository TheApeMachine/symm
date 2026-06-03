package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestSetStagnationWindow(t *testing.T) {
	convey.Convey("Given a tree search and beam width", t, func() {
		search := &TreeSearch{}
		search.SetStagnationWindow(8)

		convey.Convey("It should scale stagnation detection with beam width", func() {
			convey.So(search.stagnationWindow, convey.ShouldEqual, 8)
		})
	})
}

func TestScoreBranches(t *testing.T) {
	convey.Convey("Given replayable branches", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
		}
		search := NewHybridTreeSearch(
			ctx,
			&Profile{ctx: ctx},
			rows,
			GuardOptions{},
			nil,
			MCTSOptions{Iterations: 1},
			nil,
		)

		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := search.scoreBranches(branches)

		convey.Convey("It should score branches through replay simulation", func() {
			convey.So(score, convey.ShouldBeGreaterThan, -1)
		})
	})
}

func TestNormalizeMCTSRewardFromRollout(t *testing.T) {
	convey.Convey("Given a reward scale", t, func() {
		search := &TreeSearch{rewardScale: 2}

		convey.Convey("It should center zero PnL at 0.5", func() {
			convey.So(search.normalizeMCTSReward(0), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})
	})
}
