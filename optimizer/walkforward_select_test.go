package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCoOccurrenceIndexMinSupport(t *testing.T) {
	convey.Convey("Given a chain that co-occurs on only one tick", t, func() {
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
		}
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 0)

		convey.Convey("It should reject statistically insignificant chains", func() {
			convey.So(index.ChainReachable([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}), convey.ShouldBeFalse)
			convey.So(index.chainSupport([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}), convey.ShouldEqual, 1)
		})
	})
}

func TestSelectWalkForwardBest(t *testing.T) {
	convey.Convey("Given two finalists with different holdout quality", t, func() {
		ctx := context.Background()
		rows := make([]perspectives.Measurement, 0, 120)

		for index := range 120 {
			price := 100.0

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
				SNR:      2, Last: price,
			})
		}

		strong := perspectives.BranchList{
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
		weak := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       99, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionLimit},
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
		}, PrecompileTape(rows), nil)
		candidates := []CandidateScore{
			{AdjustedScore: 0.5, Branches: weak},
			{AdjustedScore: 0.1, Branches: strong},
		}

		selected := SelectWalkForwardBest(guard, rows, candidates)

		convey.Convey("It should prefer the stronger holdout tree over higher IS score", func() {
			convey.So(len(selected), convey.ShouldBeGreaterThan, 0)
			convey.So(selected[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(selected[0].Value, convey.ShouldAlmostEqual, 1, 0.0001)
		})
	})
}
