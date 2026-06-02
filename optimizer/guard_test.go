package optimizer

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestValidateWalkForwardUsesTrainWindow(t *testing.T) {
	convey.Convey("Given stable per-trade performance across windows", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 120)

		for index := range 120 {
			price := 100.0

			if index%6 == 5 {
				price = 110.0
			}

			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: price,
			})
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
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		guard := NewOverfitGuard(ctx, GuardOptions{
			WalkForward: WalkForwardOptions{
				Enabled:         true,
				TrainFraction:   0.7,
				TestFraction:    0.1,
				StepFraction:    0.1,
				MinWinRate:      0.5,
				MaxHoldoutDecay: 0.9,
			},
		}, PrecompileTape(rows))

		ok, _ := guard.ValidateWalkForward(branches, rows)

		convey.Convey("It should not reject solely because the test window is shorter", func() {
			convey.So(ok, convey.ShouldBeTrue)
		})
	})
}

func TestPerturbBranchValue(t *testing.T) {
	convey.Convey("Given a threshold near zero", t, func() {
		convey.Convey("It should apply a meaningful absolute shift", func() {
			convey.So(perturbBranchValue(0.001, 0.05), convey.ShouldAlmostEqual, 0.051, 0.0001)
		})
	})
}

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
		}, ReplayTape{})
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
		guard := NewOverfitGuard(ctx, GuardOptions{MinRoundTrips: 1}, PrecompileTape(rows))

		convey.Convey("It should accept profitable round trips", func() {
			convey.So(guard.AcceptTrainCandidate(branches), convey.ShouldBeTrue)
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
			at := start.Add(time.Duration(index) * time.Hour)
			rows[index] = perspectives.Measurement{
				At: at,
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
				robustUnderJitter(
					ctx, branches, PrecompileTape(rows), []float64{-0.02, 0.02}, baseline,
				),
				convey.ShouldBeTrue,
			)
		})
	})
}

func TestPersistCandidateRejectsNegativeProfit(t *testing.T) {
	convey.Convey("Given a losing but active replay tree", t, func() {
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
				SNR:      2, Last: 95,
			},
		}
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNone,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Branches: []perspectives.Branch{{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
					Condition:   perspectives.ConditionIsGreaterThanOrEqual,
					Unit:        perspectives.UnitSNR,
					Value:       1, ValueSet: true,
					Action: perspectives.Action{Type: perspectives.ActionLimit},
				}},
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
		guard := NewOverfitGuard(ctx, GuardOptions{}, PrecompileTape(rows))

		convey.Convey("It should reject persistence without positive profit", func() {
			convey.So(guard.PersistCandidate(branches), convey.ShouldBeFalse)
			convey.So(guard.AcceptTrainCandidate(branches), convey.ShouldBeFalse)
		})
	})
}

func TestImprovesPersistedBest(t *testing.T) {
	convey.Convey("Given an inert zero-return baseline", t, func() {
		guard := NewOverfitGuard(context.Background(), GuardOptions{}, ReplayTape{})

		convey.Convey("It should reject another inert candidate", func() {
			convey.So(
				guard.ImprovesPersistedBest(0, 0, 0, 0),
				convey.ShouldBeFalse,
			)
		})

		convey.Convey("It should reject a losing active candidate", func() {
			convey.So(
				guard.ImprovesPersistedBest(-0.02, 1, 0, 0),
				convey.ShouldBeFalse,
			)
		})
	})

	convey.Convey("Given an active negative best", t, func() {
		guard := NewOverfitGuard(context.Background(), GuardOptions{}, ReplayTape{})

		convey.Convey("It should require a profitable score to replace it", func() {
			convey.So(
				guard.ImprovesPersistedBest(-0.03, 1, -0.02, 1),
				convey.ShouldBeFalse,
			)
			convey.So(
				guard.ImprovesPersistedBest(-0.01, 1, -0.02, 1),
				convey.ShouldBeFalse,
			)
			convey.So(
				guard.ImprovesPersistedBest(0.01, 1, -0.02, 1),
				convey.ShouldBeTrue,
			)
		})
	})
}

func TestScanSearchIgnoresInertZeroReturn(t *testing.T) {
	convey.Convey("Given an inert candidate before active losers", t, func() {
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
				SNR:      2, Last: 95,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		bestScores := make([]float64, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           1,
			MaxThresholds:     2,
			BeamWidth:         4,
			CandidateLimit:    64,
			MaxReasoningSteps: 1,
		})
		search.onBest = func(best BestTree) {
			bestScores = append(bestScores, best.Score)
		}
		search.Run()

		convey.Convey("It should not lock YAML to an inert 0% return", func() {
			for _, score := range bestScores {
				convey.So(score, convey.ShouldNotEqual, 0)
			}
		})
	})
}

func TestScanSearchOnBestRequiresProfit(t *testing.T) {
	convey.Convey("Given a losing replay tape", t, func() {
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
				SNR:      2, Last: 95,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		bestCount := 0
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     2,
			BeamWidth:         4,
			CandidateLimit:    64,
			MaxReasoningSteps: 2,
		})
		search.onBest = func(best BestTree) {
			bestCount++
		}
		search.Run()

		convey.Convey("It should not persist a losing tree", func() {
			convey.So(bestCount, convey.ShouldEqual, 0)
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
	guard := NewOverfitGuard(context.Background(), GuardOptions{}, ReplayTape{})
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
