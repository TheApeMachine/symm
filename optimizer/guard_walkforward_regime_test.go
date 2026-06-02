package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

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

		guard := NewOverfitGuard(ctx, GuardOptions{
			WalkForward: WalkForwardOptions{MaxHoldoutDecay: 0.9},
		}, PrecompileTape(rows), nil)

		window := IndexWindow{TrainStart: 0, TrainEnd: 84, TestStart: 84, TestEnd: 120}
		tags := TagRowRegimes(rows)
		win, perTrade := guard.evaluateChronologicalWindow(branches, rows, tags, window)

		convey.Convey("It should accept stable chronological holdout performance", func() {
			convey.So(win, convey.ShouldBeTrue)
			convey.So(perTrade, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegimeSetInRange(t *testing.T) {
	convey.Convey("Given regime tags across a window", t, func() {
		tags := []StructuralRegime{
			StructuralRegimeNormalFlow,
			StructuralRegimeNormalFlow,
			StructuralRegimeLiquidityPanic,
			StructuralRegimeLiquidityPanic,
		}

		regimes := regimeSetInRange(tags, 0, 4)

		convey.Convey("It should collect distinct regimes in the slice", func() {
			convey.So(len(regimes), convey.ShouldEqual, 2)
			_, hasNormal := regimes[StructuralRegimeNormalFlow]
			_, hasPanic := regimes[StructuralRegimeLiquidityPanic]
			convey.So(hasNormal, convey.ShouldBeTrue)
			convey.So(hasPanic, convey.ShouldBeTrue)
		})
	})
}
