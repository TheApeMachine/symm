package replay

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

		withFees := NewReplaySimulationWithCosts(ctx, branches, rows, ReplayCosts{
			MakerFeePct: 0.004,
			TakerFeePct: 0.004,
			SlippagePct: DefaultSlippageBps / 10000.0,
		}).Score()
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
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2.0,
				Last:     110,
			},
		}

		branches := perspectives.BranchList{
			{
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

		convey.Convey("It should score realized profit from completed round trips", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestReplaySimulationDynamicSlippage(t *testing.T) {
	convey.Convey("Given a wide contemporaneous spread", t, func() {
		ctx := context.Background()
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		wideRows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100, SpreadBPS: 40,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 100.20, SpreadBPS: 40,
			},
		}
		tightRows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100, SpreadBPS: 2,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 100.20, SpreadBPS: 2,
			},
		}

		wideSpread := NewReplaySimulation(ctx, branches, wideRows).Score()
		tightSpread := NewReplaySimulation(ctx, branches, tightRows).Score()

		convey.Convey("It should charge more drag when the tape spread is wider", func() {
			convey.So(wideSpread, convey.ShouldBeLessThan, tightSpread)
		})
	})
}

func TestReplaySimulationReentryCooldown(t *testing.T) {
	convey.Convey("Given alternating entry and exit signals", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 1200)

		for index := range 1200 {
			price := 100.0

			if index%2 == 1 {
				price = 101.0
			}

			rows = append(rows, perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2,
				Last:     price,
			})
		}

		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		result := NewReplaySimulation(ctx, branches, rows).Result()

		convey.Convey("It should suppress immediate re-entry churn", func() {
			convey.So(result.ClosedTrades, convey.ShouldBeGreaterThan, 0)
			convey.So(result.ClosedTrades, convey.ShouldBeLessThan, 120)
		})

		convey.Convey("It should allow first entry after a delayed buy signal", func() {
			delayedRows := make([]perspectives.Measurement, 0, 1200)

			for index := range 1200 {
				price := 100.0

				if index%2 == 1 {
					price = 101.0
				}

				snr := 0.0

				if index >= 8 {
					snr = 2
				}

				delayedRows = append(delayedRows, perspectives.Measurement{
					Symbol:   "BTC/EUR",
					Source:   perspectives.SourceFluid,
					Category: perspectives.CategoryLaminar,
					SNR:      snr,
					Last:     price,
				})
			}

			delayedResult := NewReplaySimulation(ctx, branches, delayedRows).Result()

			convey.So(delayedResult.ClosedTrades, convey.ShouldBeGreaterThan, 0)
			convey.So(delayedResult.ClosedTrades, convey.ShouldBeLessThan, 120)
		})
	})
}

func TestReplaySimulationScoreIsRealizedOnly(t *testing.T) {
	convey.Convey("Given an entry-only playbook and rising prices", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategorySystemicBeta,
				SNR:      2.0, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategorySystemicBeta,
				SNR:      2.0, Last: 150,
			},
		}
		branches := perspectives.BranchList{{
			Category:    perspectives.CategorySystemicBeta,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1.0,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}

		result := NewReplaySimulationWithCosts(
			ctx, branches, rows, ReplayCosts{},
		).Result()

		convey.Convey("It should not score open inventory at end of tape", func() {
			convey.So(result.ClosedTrades, convey.ShouldEqual, 0)
			convey.So(result.Score, convey.ShouldEqual, 0)
		})
	})
}

func BenchmarkReplaySimulationResult(b *testing.B) {
	ctx := context.Background()
	rows := make([]perspectives.Measurement, 512)

	for index := range rows {
		rows[index] = perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index%4 + 1),
			Last:     100 + float64(index),
		}
	}

	branches := perspectives.BranchList{{
		Category:    perspectives.CategoryLaminar,
		Observation: perspectives.ObservationNotHolding,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       1,
		ValueSet:    true,
		Action:      perspectives.Action{Type: perspectives.ActionLimit},
	}}
	tape := PrecompileTape(rows)
	simulation := NewReplaySimulationWithTape(ctx, branches, tape)

	b.ReportAllocs()

	for b.Loop() {
		_ = simulation.Result()
	}
}
