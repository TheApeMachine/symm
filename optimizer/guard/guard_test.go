package guard

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestValidateWalkForwardUsesTrainWindow(t *testing.T) {
	convey.Convey("Given stable per-trade performance across windows", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 120)

		for index := range 120 {
			if index%6 == 5 {
				rows = append(rows, perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
					Category: perspectives.CategoryExhaustion,
					SNR:      2, Last: 200,
				})

				continue
			}

			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
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
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		guard := NewOverfitGuard(ctx, types.GuardOptions{
			WalkForward: types.WalkForwardOptions{
				Enabled:         true,
				TrainFraction:   0.7,
				TestFraction:    0.1,
				StepFraction:    0.1,
				MinWinRate:      0.5,
				MaxHoldoutDecay: 0.9,
			},
		}, replay.PrecompileTape(rows), nil)

		ok, _ := guard.ValidateWalkForward(branches, rows)

		convey.Convey("It should not reject solely because the test window is shorter", func() {
			convey.So(ok, convey.ShouldBeTrue)
		})
	})
}

func TestPerturbBranchValue(t *testing.T) {
	convey.Convey("Given a threshold near zero with a wide SNR distribution", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      0.001,
		})
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      2,
		})
		profile.PrepareCache()
		scale := profile.JitterScale(
			perspectives.CategoryLaminar,
			perspectives.UnitSNR,
			0.001,
		)

		convey.Convey("It should perturb relative to the observed IQR", func() {
			convey.So(scale, convey.ShouldAlmostEqual, 1.999, 0.001)
			convey.So(
				perturbBranchValue(0.001, 0.05, scale),
				convey.ShouldAlmostEqual,
				0.001+0.05*scale,
				0.0001,
			)
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
			convey.So(playbook.ReasoningDepth(branches), convey.ShouldEqual, 2)
		})
	})
}

func TestOverfitGuardAdjustedScore(t *testing.T) {
	convey.Convey("Given equal profit with selective shallow vs extreme deep gates", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)

		for index := range 100 {
			snr := 0.01

			if index >= 50 {
				snr = 2
			}

			profile.Add(perspectives.Measurement{
				Category: perspectives.CategoryLaminar,
				SNR:      snr,
			})
		}

		profile.PrepareCache()
		guard := NewOverfitGuard(ctx, types.GuardOptions{
			ComplexityPenalty: 0.05,
		}, replay.ReplayTape{}, profile)
		shallow := perspectives.BranchList{{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}}
		deep := perspectives.BranchList{{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     0.01,
			ValueSet:  true,
			Branches: []perspectives.Branch{{
				Category:  perspectives.CategoryLaminar,
				Condition: perspectives.ConditionIsGreaterThanOrEqual,
				Unit:      perspectives.UnitSNR,
				Value:     0.01,
				ValueSet:  true,
			}},
		}}

		convey.Convey("It should prefer the shallower selective tree", func() {
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
		guard := NewOverfitGuard(ctx, types.GuardOptions{MinRoundTrips: 1}, replay.PrecompileTape(rows), nil)

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
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      3, Last: 110,
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
		baseline := replay.NewReplaySimulation(ctx, branches, rows).Result().Score

		convey.Convey("It should survive small threshold perturbations", func() {
			profile := profile.NewProfile(ctx)
			profile.Add(rows[0])
			profile.Add(rows[1])
			profile.PrepareCache()

			convey.So(
				robustUnderJitter(
					ctx,
					branches,
					replay.PrecompileTape(rows),
					[]float64{-0.02, 0.02},
					baseline,
					profile,
				),
				convey.ShouldBeTrue,
			)
		})
	})
}

func TestPersistCandidateAcceptsNegativeProfitWithTrades(t *testing.T) {
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
		guard := NewOverfitGuard(ctx, types.GuardOptions{}, replay.PrecompileTape(rows), nil)

		convey.Convey("It should persist when round trips closed even at a loss", func() {
			convey.So(guard.PersistCandidate(branches), convey.ShouldBeTrue)
			convey.So(guard.AcceptTrainCandidate(branches), convey.ShouldBeFalse)
		})
	})
}

func TestImprovesPersistedBest(t *testing.T) {
	convey.Convey("Given an inert zero-return baseline", t, func() {
		guard := NewOverfitGuard(context.Background(), types.GuardOptions{}, replay.ReplayTape{}, nil)

		convey.Convey("It should reject another inert candidate", func() {
			convey.So(
				guard.ImprovesPersistedBest(0, 0, 0, 0),
				convey.ShouldBeFalse,
			)
		})

		convey.Convey("It should accept the first active candidate", func() {
			convey.So(
				guard.ImprovesPersistedBest(-0.02, 1, math.Inf(-1), -1),
				convey.ShouldBeTrue,
			)
		})
	})

	convey.Convey("Given an active negative best", t, func() {
		guard := NewOverfitGuard(context.Background(), types.GuardOptions{}, replay.ReplayTape{}, nil)

		convey.Convey("It should promote a less negative score", func() {
			convey.So(
				guard.ImprovesPersistedBest(-0.01, 1, -0.02, 1),
				convey.ShouldBeTrue,
			)
			convey.So(
				guard.ImprovesPersistedBest(-0.03, 1, -0.02, 1),
				convey.ShouldBeFalse,
			)
			convey.So(
				guard.ImprovesPersistedBest(0.01, 1, -0.02, 1),
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
			convey.So(playbook.IsBranchCompatible(parent, child), convey.ShouldBeFalse)
		})
	})
}

func TestEvaluateChronologicalWindow(t *testing.T) {
	convey.Convey("Given profitable train and test slices", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 120)

		for index := range 120 {
			if index%6 == 5 {
				rows = append(rows, perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
					Category: perspectives.CategoryExhaustion,
					SNR:      2, Last: 200,
				})

				continue
			}

			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
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
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}

		guard := NewOverfitGuard(ctx, types.GuardOptions{
			WalkForward: types.WalkForwardOptions{MaxHoldoutDecay: 0.9},
		}, replay.PrecompileTape(rows), nil)

		window := IndexWindow{TrainStart: 0, TrainEnd: 84, TestStart: 84, TestEnd: 120}
		tags := budget.TagRowRegimes(rows)
		win, perTrade := guard.evaluateChronologicalWindow(branches, rows, tags, window)

		convey.Convey("It should accept stable chronological holdout performance", func() {
			convey.So(win, convey.ShouldBeTrue)
			convey.So(perTrade, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegimeSetInRange(t *testing.T) {
	convey.Convey("Given regime tags across a window", t, func() {
		tags := []budget.StructuralRegime{
			budget.StructuralRegimeNormalFlow,
			budget.StructuralRegimeNormalFlow,
			budget.StructuralRegimeLiquidityPanic,
			budget.StructuralRegimeLiquidityPanic,
		}

		regimes := budget.RegimeSetInRange(tags, 0, 4)

		convey.Convey("It should collect distinct regimes in the slice", func() {
			convey.So(len(regimes), convey.ShouldEqual, 2)
			_, hasNormal := regimes[budget.StructuralRegimeNormalFlow]
			_, hasPanic := regimes[budget.StructuralRegimeLiquidityPanic]
			convey.So(hasNormal, convey.ShouldBeTrue)
			convey.So(hasPanic, convey.ShouldBeTrue)
		})
	})
}

func TestBinaryEntropyAndInformationGain(t *testing.T) {
	convey.Convey("Given balanced and separated win/loss splits", t, func() {
		balanced := replay.GatePathStats{BeforeWins: 5, BeforeLoss: 5, AfterWins: 5, AfterLoss: 5}
		separated := replay.GatePathStats{BeforeWins: 5, BeforeLoss: 5, AfterWins: 8, AfterLoss: 2}

		convey.Convey("It should report higher information gain for separated outcomes", func() {
			convey.So(separated.InformationGainBits(), convey.ShouldBeGreaterThan, balanced.InformationGainBits())
		})
	})
}

func TestInformationGainSignificant(t *testing.T) {
	convey.Convey("Given enough separated trades", t, func() {
		stats := replay.GatePathStats{
			BeforeWins: 10,
			BeforeLoss: 10,
			AfterWins:  16,
			AfterLoss:  4,
		}

		convey.Convey("It should waive complexity when gain clears the confidence bar", func() {
			convey.So(replay.InformationGainSignificant(stats, 4), convey.ShouldBeTrue)
		})
	})
}

func TestCollectGateReplayStats(t *testing.T) {
	convey.Convey("Given a replay tape with selective entry and exit", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 105,
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
		tape := replay.PrecompileTape(rows)
		collector := replay.CollectGateReplayStats(ctx, tape, branches)
		entryGate := branches[0]
		stats := collector.StatsFor(entryGate)

		convey.Convey("It should derive tape pass rate and trade outcomes from replay", func() {
			convey.So(stats.TapeBefore, convey.ShouldBeGreaterThan, 0)
			convey.So(stats.TapePasses, convey.ShouldBeGreaterThan, 0)
			convey.So(stats.AfterWins+stats.AfterLoss, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkAdaptiveComplexityPenalty(b *testing.B) {
	measurementProfile := profile.NewProfile(context.Background())

	for index := range 200 {
		measurementProfile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 5),
		})
	}

	measurementProfile.PrepareCache()
	ctx := context.Background()
	rows := measurementProfile.Rows()
	tape := replay.PrecompileTape(rows)
	overfitGuard := NewOverfitGuard(ctx, types.GuardOptions{
		ComplexityPenalty: 0.05,
	}, tape, measurementProfile)
	branches := perspectives.BranchList{{
		Category:  perspectives.CategoryLaminar,
		Condition: perspectives.ConditionIsGreaterThanOrEqual,
		Unit:      perspectives.UnitSNR,
		Value:     1,
		ValueSet:  true,
		Branches: []perspectives.Branch{{
			Category:  perspectives.CategoryExhaustion,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = overfitGuard.adaptiveComplexityPenalty(branches)
	}
}

func BenchmarkOverfitGuardAdjustedScore(b *testing.B) {
	overfitGuard := NewOverfitGuard(context.Background(), types.GuardOptions{}, replay.ReplayTape{}, nil)
	branches := perspectives.BranchList{{
		Category: perspectives.CategoryLaminar,
		Branches: []perspectives.Branch{
			{Category: perspectives.CategoryExhaustion},
		},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = overfitGuard.AdjustedScore(0.25, branches)
	}
}
