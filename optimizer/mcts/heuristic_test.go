package mcts

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestHeuristicMoveReachable(t *testing.T) {
	convey.Convey("Given a profile with laminar measurements", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		search := NewHybridTreeSearch(ctx, profile, profile.Rows(), types.GuardOptions{}, nil, Options{
			Iterations: 1,
		}, nil)

		reachable := search.heuristic.MoveReachable(Move{
			category:  perspectives.CategoryLaminar,
			unit:      perspectives.UnitSNR,
			condition: perspectives.ConditionIsGreaterThanOrEqual,
			value:     1,
		}, nil)
		unreachable := search.heuristic.MoveReachable(Move{
			category:  perspectives.CategoryToxicBluff,
			unit:      perspectives.UnitSNR,
			condition: perspectives.ConditionIsGreaterThanOrEqual,
			value:     1,
		}, nil)

		convey.Convey("It should accept categories present on the tape", func() {
			convey.So(reachable, convey.ShouldBeTrue)
			convey.So(unreachable, convey.ShouldBeFalse)
		})
	})
}

func TestHeuristicMoveWeightForBranches(t *testing.T) {
	convey.Convey("Given two moves with different pass rates", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)

		for index, snr := range []float64{1, 2, 3, 4, 5, 6} {
			profile.Add(perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      snr,
				Last:     100 + float64(index),
			})
		}

		profile.PrepareCache()

		search := NewHybridTreeSearch(ctx, profile, profile.Rows(), types.GuardOptions{}, nil, Options{
			Iterations: 1,
		}, nil)

		heavy := Move{
			category:  perspectives.CategoryLaminar,
			unit:      perspectives.UnitSNR,
			condition: perspectives.ConditionIsGreaterThanOrEqual,
			value:     2,
		}
		light := Move{
			category:  perspectives.CategoryLaminar,
			unit:      perspectives.UnitSNR,
			condition: perspectives.ConditionIsGreaterThanOrEqual,
			value:     4,
		}

		heavyWeight := search.heuristic.moveWeightForBranches(heavy, nil)
		lightWeight := search.heuristic.moveWeightForBranches(light, nil)

		convey.Convey("It should weight selective gates higher", func() {
			convey.So(heavyWeight, convey.ShouldBeGreaterThan, 0)
			convey.So(lightWeight, convey.ShouldBeGreaterThan, 0)
			convey.So(lightWeight, convey.ShouldBeGreaterThan, heavyWeight)
		})
	})
}
