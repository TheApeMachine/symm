package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestScanSearchRunTopK(t *testing.T) {
	convey.Convey("Given replay measurements with trade opportunities", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 105,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     2,
			BeamWidth:         8,
			CandidateLimit:    64,
			MaxReasoningSteps: 2,
		})
		top, stats := search.RunTopK(3)

		convey.Convey("It should return ranked shallow survivors", func() {
			convey.So(stats.Candidates, convey.ShouldBeGreaterThan, 0)
			convey.So(len(top), convey.ShouldBeGreaterThan, 0)
			convey.So(len(top), convey.ShouldBeLessThanOrEqualTo, 3)

			if len(top) > 1 {
				convey.So(top[0].Score, convey.ShouldBeGreaterThanOrEqualTo, top[1].Score)
			}
		})
	})
}

func TestRunHybridSearch(t *testing.T) {
	convey.Convey("Given replay measurements", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 105,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		profile.PrepareCache()
		tape := PrecompileTape(rows)

		branches, stats, err := RunHybridSearch(ctx, &profile, rows, tape, HybridOptions{
			ScanOptions: ScanOptions{
				Workers:           2,
				MaxThresholds:     2,
				BeamWidth:         4,
				CandidateLimit:    32,
				MaxReasoningSteps: 4,
			},
			MCTSOptions: MCTSOptions{
				Iterations:        16,
				MaxReasoningSteps: 4,
			},
			SeedCount: 4,
		})

		convey.Convey("It should deepen beam seeds with MCTS", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(stats.Scan.Candidates, convey.ShouldBeGreaterThan, 0)
			convey.So(stats.MCTSRounds, convey.ShouldEqual, 16)
			convey.So(len(branches), convey.ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestHybridTreeSearchSeeds(t *testing.T) {
	convey.Convey("Given beam survivors", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2, Last: 100,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2, Last: 110,
		})

		rows := profile.Rows()
		seeds := []CandidateScore{{
			Score: 0.05,
			Branches: perspectives.BranchList{{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1,
				ValueSet:    true,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			}},
		}}

		search := NewHybridTreeSearch(ctx, &profile, rows, GuardOptions{}, seeds, MCTSOptions{
			Iterations:        8,
			MaxReasoningSteps: 4,
		}, nil)

		convey.Convey("It should pre-populate the MCTS root with seeds", func() {
			convey.So(len(search.root.children), convey.ShouldEqual, 1)
			convey.So(search.root.children[0].visits, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkRunHybridSearch(b *testing.B) {
	ctx := context.Background()
	profile := Profile{ctx: ctx}
	rows := make([]perspectives.Measurement, 32)

	for index := range rows {
		rows[index] = perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index%4 + 1),
			Last:     100 + float64(index),
		}
		profile.Add(rows[index])
	}

	options := HybridOptions{
		ScanOptions: ScanOptions{
			Workers:           2,
			MaxThresholds:     2,
			BeamWidth:         4,
			CandidateLimit:    32,
			MaxReasoningSteps: 4,
		},
		MCTSOptions: MCTSOptions{
			Iterations:        8,
			MaxReasoningSteps: 4,
		},
		SeedCount: 4,
	}
	tape := PrecompileTape(rows)

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = RunHybridSearch(ctx, &profile, rows, tape, options)
	}
}
