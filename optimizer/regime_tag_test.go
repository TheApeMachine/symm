package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTagRowRegimes(t *testing.T) {
	convey.Convey("Given causal measurements on replay rows", t, func() {
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 3, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryLiquidityShock, SNR: 5, Last: 90,
			},
		}
		tags := TagRowRegimes(rows)

		convey.Convey("It should tag each row by dominant causal regime", func() {
			convey.So(tags[0], convey.ShouldEqual, StructuralRegimeNormalFlow)
			convey.So(tags[1], convey.ShouldEqual, StructuralRegimeLiquidityPanic)
		})
	})
}

func TestEvaluateRegimeStratifiedWindowSkipsUnprecedentedShift(t *testing.T) {
	convey.Convey("Given a test slice with an unseen panic regime", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 40)

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
				Category: perspectives.CategoryLiquidityShock, SNR: 5, Last: 80,
			})
			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 80,
			})
		}

		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}
		guard := NewOverfitGuard(ctx, GuardOptions{
			WalkForward: WalkForwardOptions{
				Enabled:         true,
				TrainFraction:   0.6,
				TestFraction:    0.2,
				StepFraction:    0.2,
				MaxHoldoutDecay: 0.9,
			},
		}, PrecompileTape(rows), nil)
		tags := TagRowRegimes(rows)
		window := IndexWindow{
			TrainStart: 0,
			TrainEnd:   48,
			TestStart:  48,
			TestEnd:    80,
		}

		convey.Convey("It should pause regime holdout decay on unprecedented shifts", func() {
			win, _ := guard.evaluateRegimeStratifiedWindow(branches, rows, tags, window)
			convey.So(win, convey.ShouldBeFalse)
		})
	})
}
