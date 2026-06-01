package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplaySimulationScore(t *testing.T) {
	convey.Convey("Given a profile with rising prices", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2.0,
				Last:     100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2.5,
				Last:     110,
			},
		}

		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThan,
			Unit:        perspectives.UnitSNR,
			Value:       1.0,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should reward profitable entry branches", func() {
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
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2.5,
				Last:     110,
			},
		}
		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       2.0,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := trader.Evaluate(branches, rows)

		convey.Convey("It should replay the full measurement stream", func() {
			convey.So(score, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTreeSearchRun(t *testing.T) {
	convey.Convey("Given replay measurements", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.0,
			Last:     100,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.5,
			Last:     110,
		})

		tuner := &Tuner{ctx: ctx, profile: profile, seed: 1}
		search := tuner.newTreeSearch()
		search.iterations = 32
		branches := search.Run()

		convey.Convey("It should return a branch registry", func() {
			convey.So(len(branches), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTreeSearchRunKeepsEmptyTreeWhenTradingLoses(t *testing.T) {
	convey.Convey("Given replay measurements with falling prices", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.0,
			Last:     100,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.5,
			Last:     90,
		})

		tuner := &Tuner{ctx: ctx, profile: profile, seed: 1}
		search := tuner.newTreeSearch()
		search.iterations = 32
		branches := search.Run()

		convey.Convey("It should not choose a losing trade over no trade", func() {
			convey.So(len(branches), convey.ShouldEqual, 0)
		})
	})
}

func TestTreeSearchMoves(t *testing.T) {
	convey.Convey("Given generated optimizer moves", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		tuner := &Tuner{ctx: ctx, profile: profile, seed: 1}
		search := tuner.newTreeSearch()
		moves := search.moves(perspectives.BranchList{})

		convey.Convey("It should only attach state-coherent actions", func() {
			for _, move := range moves {
				convey.So(move.validActionForObservation(), convey.ShouldBeTrue)
			}
		})
	})
}

func TestScanSearchRun(t *testing.T) {
	convey.Convey("Given replay measurements with entry and exit opportunities", t, func() {
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

		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:        2,
			MaxThresholds:  2,
			BeamWidth:      8,
			CandidateLimit: 512,
		})
		branches, stats := search.Run()
		score := NewReplaySimulation(ctx, branches, rows).Score()

		convey.Convey("It should score a positive bounded scan candidate", func() {
			convey.So(stats.Candidates, convey.ShouldBeGreaterThan, 0)
			convey.So(len(branches), convey.ShouldBeGreaterThan, 0)
			convey.So(score, convey.ShouldBeGreaterThan, 0)
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
			seed:    1,
		}

		tuner.ingest(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.0,
			Last:     100,
		})
		tuner.ingest(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2.5,
			Last:     110,
		})
		tuner.Finish()

		summary := tuner.Summary()

		convey.Convey("It should search branches after replay ends", func() {
			convey.So(summary.MeasurementCount, convey.ShouldEqual, 2)
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

func (move Move) validActionForObservation() bool {
	switch move.observation {
	case perspectives.ObservationNone:
		return move.action == perspectives.ActionNone
	case perspectives.ObservationNotHolding:
		return move.action == perspectives.ActionNone ||
			move.action == perspectives.ActionLimit ||
			move.action == perspectives.ActionMarket ||
			move.action == perspectives.ActionIceberg
	case perspectives.ObservationHolding:
		return move.action == perspectives.ActionNone ||
			move.action == perspectives.ActionStopLoss ||
			move.action == perspectives.ActionStopLossLimit ||
			move.action == perspectives.ActionTakeProfit ||
			move.action == perspectives.ActionTakeProfitLimit ||
			move.action == perspectives.ActionTrailingStop ||
			move.action == perspectives.ActionTrailingStopLimit ||
			move.action == perspectives.ActionSettlePosition
	default:
		return false
	}
}

func BenchmarkTreeSearchRun(b *testing.B) {
	ctx := context.Background()
	profile := Profile{ctx: ctx}

	for index := range 64 {
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 8),
			Last:     100 + float64(index),
		})
	}

	tuner := &Tuner{ctx: ctx, profile: profile, seed: 1}
	search := tuner.newTreeSearch()
	search.iterations = 64

	b.ReportAllocs()

	for b.Loop() {
		_ = search.Run()
	}
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

	for b.Loop() {
		_ = NewReplaySimulation(ctx, branches, rows).Score()
	}
}
