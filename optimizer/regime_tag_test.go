package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTagRowRegimes(t *testing.T) {
	convey.Convey("Given causal measurements on replay rows", t, func() {
		viper.Set("signals.causal.condition_switch", 100.0)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 3, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryLiquidityShock,
				Strength: 150, SNR: 5, Last: 90,
			},
		}
		tags := TagRowRegimes(rows)

		convey.Convey("It should tag each row by dominant causal regime", func() {
			convey.So(tags[0], convey.ShouldEqual, StructuralRegimeNormalFlow)
			convey.So(tags[1], convey.ShouldEqual, StructuralRegimeLiquidityPanic)
		})
	})
}

func TestEvaluateChronologicalWindowPausesOnRegimeShift(t *testing.T) {
	convey.Convey("Given a chronological test slice with an unseen panic regime", t, func() {
		ctx := context.Background()
		rows, tags := walkForwardRegimeFixture()
		branches := laminarEntryBranch()
		guard := walkForwardGuard(ctx, rows)
		window := IndexWindow{TrainStart: 0, TrainEnd: 48, TestStart: 48, TestEnd: 80}

		convey.Convey("It should pause chronological holdout decay", func() {
			win, _ := guard.evaluateChronologicalWindow(branches, rows, tags, window)
			convey.So(win, convey.ShouldBeFalse)
		})
	})
}

func TestEvaluateRegimeStratifiedWindowMatchesRegimePairs(t *testing.T) {
	convey.Convey("Given normal-flow train and test slices with round trips", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 120)

		for index := range 40 {
			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 2, Last: 100,
			})

			if index%6 == 5 {
				rows = append(rows, perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
					Category: perspectives.CategoryExhaustion, SNR: 2, Last: 200,
				})

				continue
			}

			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			})
		}

		for index := range 40 {
			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 2, Last: 100,
			})

			if index%6 == 5 {
				rows = append(rows, perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
					Category: perspectives.CategoryExhaustion, SNR: 2, Last: 200,
				})

				continue
			}

			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			})
		}

		tags := TagRowRegimes(rows)
		guard := walkForwardGuard(ctx, rows)
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
		window := IndexWindow{TrainStart: 0, TrainEnd: 60, TestStart: 60, TestEnd: 120}

		convey.Convey("It should validate normal-flow logic on normal-flow holdout", func() {
			win, _ := guard.evaluateRegimeStratifiedWindow(
				branches, rows, tags, window,
			)
			convey.So(win, convey.ShouldBeTrue)
		})
	})
}

func walkForwardRegimeFixture() ([]perspectives.Measurement, []StructuralRegime) {
	rows := make([]perspectives.Measurement, 0, 80)

	for range 30 {
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
			Category: perspectives.CategoryEndogenousAlpha, SNR: 2, Last: 100,
		})
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
		})
	}

	for range 10 {
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
			Category: perspectives.CategoryLiquidityShock,
			Strength: 150, SNR: 5, Last: 80,
		})
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar, SNR: 2, Last: 80,
		})
	}

	return rows, TagRowRegimes(rows)
}

func laminarEntryBranch() perspectives.BranchList {
	return perspectives.BranchList{{
		Category:    perspectives.CategoryLaminar,
		Observation: perspectives.ObservationNotHolding,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       1,
		ValueSet:    true,
		Action:      perspectives.Action{Type: perspectives.ActionLimit},
	}}
}

func walkForwardGuard(
	ctx context.Context,
	rows []perspectives.Measurement,
) *OverfitGuard {
	return NewOverfitGuard(ctx, GuardOptions{
		WalkForward: WalkForwardOptions{
			Enabled:         true,
			TrainFraction:   0.6,
			TestFraction:    0.2,
			StepFraction:    0.2,
			MaxHoldoutDecay: 0.9,
		},
	}, PrecompileTape(rows), nil)
}

func init() {
	if viper.GetViper().GetFloat64("signals.causal.condition_switch") <= 0 {
		viper.Set("signals.causal.condition_switch", 100.0)
	}
}
