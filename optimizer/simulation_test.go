package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplaySimulationFeesRejectSubPercentScalps(t *testing.T) {
	convey.Convey("Given a sub-percent round trip", t, func() {
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
				SNR:      2.0, Last: 100.10,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2.0, Last: 100.10,
			},
		}
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1.0,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1.0,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		withFees := NewReplaySimulation(ctx, branches, rows).Score()
		withoutFees := NewReplaySimulationWithCosts(
			ctx, branches, rows, ReplayCosts{},
		).Score()

		convey.Convey("It should look profitable without fees and lose after fees", func() {
			convey.So(withoutFees, convey.ShouldBeGreaterThan, 0)
			convey.So(withFees, convey.ShouldBeLessThan, 0)
		})
	})
}

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
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2.0, Last: 90,
			},
		}

		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1.0,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1.0,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should realize PnL on exit actions", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestReplaySimulationScoreUsesLatestMeasurements(t *testing.T) {
	convey.Convey("Given a category that has aged out of the story ring", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, StoryRingCapacity+3)
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
			Category: perspectives.CategoryAggressiveDrive,
			SNR:      2.0,
		})

		for range StoryRingCapacity {
			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryStochasticBalance,
				SNR:      2.0,
			})
		}

		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
			Category: perspectives.CategoryStochasticBalance,
			SNR:      2.0, Last: 120,
		})

		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryAggressiveDrive,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1.0,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should not keep categories that fell out of the ring window", func() {
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
					Category:    perspectives.CategoryAggressiveDrive,
					Observation: perspectives.ObservationNotHolding,
					Condition:   perspectives.ConditionIsGreaterThanOrEqual,
					Unit:        perspectives.UnitSNR,
					Value:       1.0,
					ValueSet:    true,
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
