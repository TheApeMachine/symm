package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestChainReachabilityScoreProbabilisticDecay(t *testing.T) {
	convey.Convey("Given adjacent-but-not-simultaneous categories", t, func() {
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
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 4)
		chain := []perspectives.CategoryType{
			perspectives.CategoryLaminar,
			perspectives.CategoryExhaustion,
		}

		convey.Convey("It should decay support instead of hard pruning", func() {
			convey.So(index.ChainReachable(chain), convey.ShouldBeTrue)
			score := index.ChainReachabilityScore(chain)
			convey.So(score, convey.ShouldBeGreaterThan, 0)
			convey.So(score, convey.ShouldBeLessThan, 1)
		})
	})
}

func TestMoveReachabilityUsesReachabilityScore(t *testing.T) {
	convey.Convey("Given adjacent-but-not-simultaneous categories", t, func() {
		ctx := t.Context()
		profile := Profile{ctx: ctx}
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

		tape := PrecompileTape(rows)
		search := NewHybridTreeSearchWithTape(
			ctx, &profile, rows, tape, GuardOptions{}, nil, MCTSOptions{
				Budget: SearchBudget{
					BeamWidth:          1,
					MinChainSupport:    4,
					NearMissTickJitter: 1,
				},
			},
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
			allowed, theoretical, discount := search.moveReachability(
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
}

func TestFilterReachableEntryBranchersUsesSoftScore(t *testing.T) {
	convey.Convey("Given a near-miss nested gate", t, func() {
		profile := Profile{ctx: context.Background()}
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

		index := NewCoOccurrenceIndex(PrecompileTape(rows), 4)
		base := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		}}
		gates := []perspectives.Branch{{
			Category: perspectives.CategoryExhaustion,
		}}

		convey.Convey("It should retain beam gates with probabilistic reachability", func() {
			reachable := filterReachableEntryBranchers(index, base, gates)
			convey.So(len(reachable), convey.ShouldEqual, 1)
		})
	})
}

func init() {
	if viper.GetViper().GetFloat64("signals.causal.condition_switch") <= 0 {
		viper.Set("signals.causal.condition_switch", 100.0)
	}
}
