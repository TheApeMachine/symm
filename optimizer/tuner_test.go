package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
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

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should reward realized round-trip profit", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTraderEvaluate(t *testing.T) {
	convey.Convey("Given a trader and replay rows", t, func() {
		ctx := context.Background()
		trader := &Trader{ctx: ctx}
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

		score := trader.Evaluate(branches, rows)

		convey.Convey("It should score realized round-trip profit", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestScanSearchRun(t *testing.T) {
	convey.Convey("Given replay measurements with entry and exit opportunities", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := profitableMultiSignalRows()

		for _, row := range rows {
			profile.Add(row)
		}

		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     2,
			BeamWidth:         8,
			CandidateLimit:    512,
			MaxReasoningSteps: 2,
		})
		topK, stats := search.RunTopK(1)

		convey.Convey("It should score complete playbook candidates", func() {
			convey.So(stats.Candidates, convey.ShouldBeGreaterThan, 0)
			convey.So(len(topK), convey.ShouldBeGreaterThan, 0)
			convey.So(topK[0].BranchCount(), convey.ShouldBeGreaterThanOrEqualTo, 2)
			convey.So(topK[0].Score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestScanSearchScoresCompletePlaybooks(t *testing.T) {
	convey.Convey("Given many category thresholds and a tight candidate budget", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
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
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2,
				Last:     90,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		scored := make([]CandidateScore, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         16,
			CandidateLimit:    64,
			MaxReasoningSteps: 4,
		})
		search.onCandidate = func(candidate CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should emit complete playbooks not orphan single-action leaves", func() {
			convey.So(len(scored), convey.ShouldBeGreaterThan, 0)

			for _, candidate := range scored {
				convey.So(
					candidate.BranchCount(),
					convey.ShouldBeGreaterThanOrEqualTo,
					2,
				)
			}
		})
	})
}

func TestTunerFinish(t *testing.T) {
	convey.Convey("Given ingested replay measurements", t, func() {
		ctx := context.Background()
		trader := &Trader{ctx: ctx}
		tuner := &Tuner{
			ctx:     ctx,
			profile: Profile{ctx: ctx},
			trader:  trader,
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
		summary := SessionSummary{
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

func BenchmarkScanSearchRun(b *testing.B) {
	ctx := context.Background()
	profile := Profile{ctx: ctx}
	rows := make([]perspectives.Measurement, 64)

	for index := range rows {
		category := perspectives.CategoryLaminar

		if index%2 == 1 {
			category = perspectives.CategoryExhaustion
		}

		rows[index] = perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: category,
			SNR:      float64(index % 8),
			Last:     100 + float64(index%16),
		}
		profile.Add(rows[index])
	}

	b.ReportAllocs()

	for b.Loop() {
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:        2,
			MaxThresholds:  4,
			BeamWidth:      8,
			CandidateLimit: 512,
		})
		_, _ = search.Run()
	}
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
	tape := PrecompileTape(rows)

	for b.Loop() {
		_ = NewReplaySimulationWithTape(ctx, branches, tape).Score()
	}
}
