package optimizer

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReasoningDepth(t *testing.T) {
	convey.Convey("Given sibling denies and one nested exit", t, func() {
		branches := perspectives.BranchList{
			{Category: perspectives.CategoryToxicBluff},
			{Category: perspectives.CategoryLiquidityVacuum},
			{
				Category: perspectives.CategoryLaminar,
				Branches: []perspectives.Branch{
					{Category: perspectives.CategoryExhaustion},
				},
			},
		}

		convey.Convey("It should count nested chain depth not sibling count", func() {
			convey.So(reasoningDepth(branches), convey.ShouldEqual, 2)
		})
	})
}

func TestOverfitGuardAdjustedScore(t *testing.T) {
	convey.Convey("Given equal profit at different reasoning depth", t, func() {
		guard := NewOverfitGuard(context.Background(), GuardOptions{
			ComplexityPenalty: 0.01,
		})
		shallow := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}
		deep := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
			Branches: []perspectives.Branch{
				{Category: perspectives.CategoryExhaustion},
			},
		}}

		convey.Convey("It should prefer the shallower tree", func() {
			shallowScore := guard.AdjustedScore(0.10, shallow)
			deepScore := guard.AdjustedScore(0.10, deep)
			convey.So(shallowScore, convey.ShouldBeGreaterThan, deepScore)
		})
	})
}

func TestOverfitGuardAcceptTrainCandidate(t *testing.T) {
	convey.Convey("Given a one-trade replay tape", t, func() {
		ctx := context.Background()
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
		guard := NewOverfitGuard(ctx, GuardOptions{MinRoundTrips: 1})

		convey.Convey("It should accept profitable round trips", func() {
			convey.So(guard.AcceptTrainCandidate(branches, rows), convey.ShouldBeTrue)
		})
	})
}

func TestGenerateIndexWindows(t *testing.T) {
	convey.Convey("Given 100 chronological rows", t, func() {
		windows := GenerateIndexWindows(100, 0.7, 0.1, 0.1)

		convey.Convey("It should produce rolling train/test slices", func() {
			convey.So(len(windows), convey.ShouldBeGreaterThanOrEqualTo, 2)
			convey.So(windows[0].TrainEnd, convey.ShouldEqual, 70)
			convey.So(windows[0].TestStart, convey.ShouldEqual, 70)
			convey.So(windows[0].TestEnd, convey.ShouldEqual, 80)
		})
	})
}

func TestGenerateTimeWindows(t *testing.T) {
	convey.Convey("Given timestamped measurements", t, func() {
		start := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
		rows := make([]perspectives.Measurement, 24)

		for index := range rows {
			rows[index] = perspectives.Measurement{
				At: start.Add(time.Duration(index) * time.Hour),
			}
		}

		windows := GenerateTimeWindows(rows, 12*time.Hour, 2*time.Hour, 2*time.Hour)

		convey.Convey("It should align windows to timestamps", func() {
			convey.So(len(windows), convey.ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}

func TestRobustUnderJitter(t *testing.T) {
	convey.Convey("Given a stable threshold branch", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      3, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      3, Last: 110,
			},
		}
		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		}}
		baseline := NewReplaySimulation(ctx, branches, rows).Result().Score

		convey.Convey("It should survive small threshold perturbations", func() {
			convey.So(
				robustUnderJitter(ctx, branches, rows, []float64{-0.02, 0.02}, baseline),
				convey.ShouldBeTrue,
			)
		})
	})
}

func TestIsBranchCompatible(t *testing.T) {
	convey.Convey("Given contradictory sequential thresholds", t, func() {
		parent := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThan,
			Value:     3,
			ValueSet:  true,
		}
		child := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsLessThan,
			Value:     2,
			ValueSet:  true,
		}

		convey.Convey("It should reject impossible paths", func() {
			convey.So(isBranchCompatible(parent, child), convey.ShouldBeFalse)
		})
	})
}

func BenchmarkOverfitGuardAdjustedScore(b *testing.B) {
	guard := NewOverfitGuard(context.Background(), GuardOptions{})
	branches := perspectives.BranchList{{
		Category: perspectives.CategoryLaminar,
		Branches: []perspectives.Branch{
			{Category: perspectives.CategoryExhaustion},
		},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = guard.AdjustedScore(0.25, branches)
	}
}
