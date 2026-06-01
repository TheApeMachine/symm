package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplaySimulationExit(t *testing.T) {
	convey.Convey("Given entry and exit branches", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Category: perspectives.CategoryLaminar,
				SNR: 2.0, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Category: perspectives.CategoryExhaustion,
				SNR: 2.0, Last: 110,
			},
		}

		branches := perspectives.BranchList{
			{
				Category:  perspectives.CategoryLaminar,
				Condition: perspectives.ConditionIsGreaterThanOrEqual,
				Unit:      perspectives.UnitSNR,
				Value:     1.0,
				ValueSet:  true,
				Action:    perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:  perspectives.CategoryExhaustion,
				Condition: perspectives.ConditionIsGreaterThanOrEqual,
				Unit:      perspectives.UnitSNR,
				Value:     1.0,
				ValueSet:  true,
				Action:    perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should realize PnL on exit actions", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}
