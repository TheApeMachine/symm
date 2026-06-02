package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestMoveReachable(t *testing.T) {
	convey.Convey("Given profile rows for laminar SNR", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2, Confidence: 1,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      3, Confidence: 1,
		})

		search := NewHybridTreeSearch(ctx, &profile, profile.Rows(), GuardOptions{}, nil, MCTSOptions{
			Iterations: 1,
		})

		reachable := search.moveReachable(Move{
			category:    perspectives.CategoryLaminar,
			unit:        perspectives.UnitSNR,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			quantile:    0.5,
			observation: perspectives.ObservationNotHolding,
			action:      perspectives.ActionLimit,
		})
		unreachable := search.moveReachable(Move{
			category:    perspectives.CategoryToxicBluff,
			unit:        perspectives.UnitSNR,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			quantile:    0.5,
			observation: perspectives.ObservationNotHolding,
			action:      perspectives.ActionLimit,
		})

		convey.Convey("It should reject gates that never fire on the tape", func() {
			convey.So(reachable, convey.ShouldBeTrue)
			convey.So(unreachable, convey.ShouldBeFalse)
		})
	})
}

func TestSampleRolloutMoveWeighting(t *testing.T) {
	convey.Convey("Given moves with different gate pass rates", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}

		for range 20 {
			profile.Add(perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Confidence: 1,
			})
		}

		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
			Category: perspectives.CategoryExhaustion,
			SNR:      0.1, Confidence: 1,
		})

		search := NewHybridTreeSearch(ctx, &profile, profile.Rows(), GuardOptions{}, nil, MCTSOptions{
			Iterations: 1,
		})

		heavy := Move{
			category:    perspectives.CategoryLaminar,
			unit:        perspectives.UnitSNR,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			quantile:    0.25,
			observation: perspectives.ObservationNotHolding,
			action:      perspectives.ActionLimit,
		}
		light := Move{
			category:    perspectives.CategoryExhaustion,
			unit:        perspectives.UnitSNR,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			quantile:    0.75,
			observation: perspectives.ObservationNotHolding,
			action:      perspectives.ActionLimit,
		}

		convey.Convey("It should rank the high-pass move above the rare gate", func() {
			convey.So(search.moveWeight(heavy), convey.ShouldBeGreaterThan, search.moveWeight(light))
		})
	})
}

func TestGatePassCount(t *testing.T) {
	convey.Convey("Given laminar rows", t, func() {
		profile := Profile{}
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar, SNR: 1,
		})
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar, SNR: 3,
		})

		passes := profile.GatePassCount(
			perspectives.CategoryLaminar,
			perspectives.UnitSNR,
			perspectives.ConditionIsGreaterThanOrEqual,
			2,
		)

		convey.Convey("It should count rows that satisfy the gate", func() {
			convey.So(passes, convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkSampleRolloutMove(b *testing.B) {
	ctx := context.Background()
	profile := Profile{ctx: ctx}

	for index := range 64 {
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 8),
		})
	}

	search := NewHybridTreeSearch(ctx, &profile, profile.Rows(), GuardOptions{}, nil, MCTSOptions{})
	moves := search.allMoves()
	branches := perspectives.BranchList{}

	b.ReportAllocs()

	for b.Loop() {
		_ = search.sampleRolloutMove(moves, branches)
	}
}
