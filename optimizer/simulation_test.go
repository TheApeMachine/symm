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
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2.0, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2.0, Last: 110,
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

func TestReplaySimulationScoreUsesLatestMeasurements(t *testing.T) {
	convey.Convey("Given a category that has already been replaced", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryAggressiveDrive,
				SNR:      2.0,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryStochasticBalance,
				SNR:      2.0,
				Last:     100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryStochasticBalance,
				SNR:      2.0,
				Last:     120,
			},
		}

		branches := perspectives.BranchList{{
			Category:  perspectives.CategoryAggressiveDrive,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1.0,
			ValueSet:  true,
			Action:    perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should not keep stale categories alive", func() {
			convey.So(score, convey.ShouldEqual, 0)
		})
	})
}

func TestReplaySimulationScoreUsesGlobalMeasurements(t *testing.T) {
	convey.Convey("Given a global market measurement and symbol measurement", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Source:   perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge,
				SNR:      2.0,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryAggressiveDrive,
				SNR:      2.0,
				Last:     100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryAggressiveDrive,
				SNR:      2.0,
				Last:     110,
			},
		}

		branches := perspectives.BranchList{{
			Category:  perspectives.CategoryRiskOnSurge,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1.0,
			ValueSet:  true,
			Branches: []perspectives.Branch{
				{
					Category:  perspectives.CategoryAggressiveDrive,
					Condition: perspectives.ConditionIsGreaterThanOrEqual,
					Unit:      perspectives.UnitSNR,
					Value:     1.0,
					ValueSet:  true,
					Action: perspectives.Action{
						Type: perspectives.ActionLimit,
					},
				},
			},
		}}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should make global context available to symbol decisions", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}
