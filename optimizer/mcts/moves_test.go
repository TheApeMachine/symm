package mcts

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestMovesGenerate(t *testing.T) {
	convey.Convey("Given a hybrid tree search profile", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		search := NewHybridTreeSearch(
			ctx,
			profile,
			profile.Rows(),
			types.GuardOptions{},
			nil,
			Options{Iterations: 1},
			nil,
		)
		generated := search.Moves().Generate()

		convey.Convey("It should enumerate threshold and action moves", func() {
			convey.So(len(generated), convey.ShouldBeGreaterThan, 0)
			convey.So(search.Moves().Cached(), convey.ShouldResemble, generated)
		})
	})
}

func TestMovesApplyNestsGates(t *testing.T) {
	convey.Convey("Given a flat entry playbook", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
			Category: perspectives.CategoryRiskOnSurge, SNR: 2, Last: 100,
		})
		profile.PrepareCache()

		moves := NewMoves(profile, nil, 0, 8)
		entry := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}
		gateMove := Move{
			category:    perspectives.CategoryRiskOnSurge,
			observation: perspectives.ObservationNone,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       profile.Quantile(perspectives.CategoryRiskOnSurge, perspectives.UnitSNR, 0.5),
			action:      perspectives.ActionNone,
		}

		nested := moves.Apply(entry, gateMove)

		convey.Convey("It should nest deny gates under the entry chain", func() {
			convey.So(playbook.ReasoningDepth(nested), convey.ShouldEqual, 2)
			convey.So(nested[0].Category, convey.ShouldEqual, perspectives.CategoryRiskOnSurge)
			convey.So(nested[0].Branches[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
		})
	})
}

func TestMovesMoveReachabilityUsesReachabilityScore(t *testing.T) {
	convey.Convey("Given adjacent-but-not-simultaneous categories", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		tape := replay.PrecompileTape(rows)
		search := NewHybridTreeSearchWithTape(
			ctx, profile, rows, tape, types.GuardOptions{}, nil, Options{
				Budget: types.SearchBudget{
					BeamWidth:          1,
					MinChainSupport:    4,
					NearMissTickJitter: 1,
				},
			},
			nil,
		)
		move := Move{
			category:    perspectives.CategoryExhaustion,
			observation: perspectives.ObservationNotHolding,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       1,
			action:      perspectives.ActionLimit,
		}

		convey.Convey("It should allow theoretical moves with probabilistic UCT discount", func() {
			allowed, theoretical, discount := search.Moves().MoveReachability(
				move, perspectives.BranchList{{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
					Condition:   perspectives.ConditionIsGreaterThanOrEqual,
					Unit:        perspectives.UnitSNR,
					Value:       1,
					ValueSet:    true,
					Action:      perspectives.Action{Type: perspectives.ActionLimit},
				}},
			)

			convey.So(allowed, convey.ShouldBeTrue)
			convey.So(theoretical, convey.ShouldBeTrue)
			convey.So(discount, convey.ShouldBeGreaterThan, 0)
			convey.So(discount, convey.ShouldBeLessThan, 1)
		})
	})

	convey.Convey("Given an existing branch and a threshold move", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		moves := NewMoves(profile, nil, 0, 4)

		start := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}
		move := Move{
			category:    perspectives.CategoryLaminar,
			observation: perspectives.ObservationNotHolding,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       1,
		}

		branches := moves.Apply(start, move)

		convey.Convey("It should apply the move to the branch list", func() {
			convey.So(len(branches), convey.ShouldBeGreaterThan, 1)
			convey.So(branches[len(branches)-1].ValueSet, convey.ShouldBeTrue)
			convey.So(branches[len(branches)-1].Value, convey.ShouldEqual, 1)
		})
	})
}
