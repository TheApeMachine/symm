package tune

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

func profitableMultiSignalRows() []perspectives.Measurement {
	return []perspectives.Measurement{
		{
			Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
			Category: perspectives.CategoryRiskOnSurge,
			SNR:      2,
			Last:     100,
		},
		{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		},
		{
			Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
			Category: perspectives.CategoryExhaustion,
			SNR:      2,
			Last:     110,
		},
	}
}

func TestReplaySimulationScore(t *testing.T) {
	convey.Convey("Given a completed round trip", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
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
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThan,
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

		score := replay.NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should reward realized round-trip profit", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTraderEvaluate(t *testing.T) {
	convey.Convey("Given replay rows and branches", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
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
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       2.0,
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

		score := replay.NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should score realized round-trip profit", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTunerFinish(t *testing.T) {
	convey.Convey("Given ingested replay measurements", t, func() {
		ctx := context.Background()
		tuner := &Tuner{
			ctx:     ctx,
			profile: *profile.NewProfile(ctx),
		}

		tuner.ingest(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
			Category: perspectives.CategoryRiskOnSurge,
			SNR:      2.0,
			Last:     100,
		})
		tuner.ingest(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.0,
			Last:     100,
		})
		tuner.ingest(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
			Category: perspectives.CategoryExhaustion,
			SNR:      2.0,
			Last:     110,
		})
		tuner.Finish()

		summary := tuner.Summary()

		convey.Convey("It should search branches after replay ends", func() {
			convey.So(summary.MeasurementCount, convey.ShouldEqual, 3)
			convey.So(summary.BranchCount, convey.ShouldBeGreaterThan, 0)
			convey.So(summary.BestScore, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestSessionSummaryString(t *testing.T) {
	convey.Convey("Given a tune summary with scanned candidates", t, func() {
		summary := types.SessionSummary{
			MeasurementCount: 10,
			BranchCount:      2,
			Candidates:       1000,
			Workers:          4,
			BestScore:        0.25,
		}

		convey.Convey("It should report the candidate count", func() {
			convey.So(summary.String(), convey.ShouldContainSubstring, "candidates=1000")
			convey.So(summary.String(), convey.ShouldContainSubstring, "workers=4")
		})
	})

}

func BenchmarkReplaySimulationScore(b *testing.B) {
	ctx := context.Background()
	rows := make([]perspectives.Measurement, 64)

	for index := range rows {
		rows[index] = perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 8),
			Last:     100 + float64(index),
		}
	}

	branches := perspectives.BranchList{{
		Category:    perspectives.CategoryLaminar,
		Observation: perspectives.ObservationNotHolding,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       1.0,
		ValueSet:    true,
		Action:      perspectives.Action{Type: perspectives.ActionLimit},
	}}

	b.ReportAllocs()
	tape := replay.PrecompileTape(rows)

	for b.Loop() {
		_ = replay.NewReplaySimulationWithTape(ctx, branches, tape).Score()
	}
}
