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
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThan,
			Unit:      perspectives.UnitSNR,
			Value:     1.0,
			Action:    perspectives.Action{Type: perspectives.ActionLimit},
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
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     2.0,
			ValueSet:  true,
			Action:    perspectives.Action{Type: perspectives.ActionLimit},
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
		Category:  perspectives.CategoryLaminar,
		Condition: perspectives.ConditionIsGreaterThanOrEqual,
		Unit:      perspectives.UnitSNR,
		Value:     1.0,
		ValueSet:  true,
		Action:    perspectives.Action{Type: perspectives.ActionLimit},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = NewReplaySimulation(ctx, branches, rows).Score()
	}
}
