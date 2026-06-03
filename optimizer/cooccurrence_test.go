package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCoOccurrenceIndexChainReachable(t *testing.T) {
	convey.Convey("Given ticks where laminar and exhaustion co-exist", t, func() {
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
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 105,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 108,
			},
		}
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 0)

		convey.Convey("It should accept chains present on one snapshot", func() {
			convey.So(index.ChainReachable([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}), convey.ShouldBeTrue)
		})

		convey.Convey("It should reject chains mixing categories that never share a tick", func() {
			convey.So(index.ChainReachable([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryToxicBluff,
			}), convey.ShouldBeFalse)
		})
	})
}

func TestBuildDecisionSeedPlaybooks(t *testing.T) {
	convey.Convey("Given a tape with a DECISION.md entry chain", t, func() {
		profile := Profile{}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceHawkes,
				Category: perspectives.CategoryFrenzy,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryAggressiveDrive,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryActiveReversal,
				SNR:      2, Last: 110,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		profile.PrepareCache()
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 0)
		playbooks := BuildDecisionSeedPlaybooks(&profile, index)

		convey.Convey("It should emit nested reachable decision seeds", func() {
			convey.So(len(playbooks), convey.ShouldBeGreaterThan, 0)

			deepest := 0

			for _, playbook := range playbooks {
				depth := reasoningDepth(playbook)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThanOrEqualTo, 3)
		})
	})
}

func TestEntryExitPairReachableWithoutCoOccurrence(t *testing.T) {
	convey.Convey("Given entry and exit categories that never share a tick", t, func() {
		rows := make([]perspectives.Measurement, 0, StoryRingCapacity+2)
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2, Last: 100,
		})

		for range StoryRingCapacity {
			rows = append(rows, perspectives.Measurement{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryInertial,
				SNR:      1, Last: 100,
			})
		}

		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
			Category: perspectives.CategoryExhaustion,
			SNR:      2, Last: 110,
		})

		index := NewCoOccurrenceIndex(PrecompileTape(rows), 0)
		entry := perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}
		exit := perspectives.BranchList{{
			Category: perspectives.CategoryExhaustion,
		}}

		convey.Convey("It should still accept the pair when each path is seen independently", func() {
			convey.So(
				entryExitPairReachable(index, entry, exit),
				convey.ShouldBeTrue,
			)
			convey.So(index.CoOccur(
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			), convey.ShouldBeFalse)
		})
	})
}

func TestNestedEntryGateReachableWithoutExitCategory(t *testing.T) {
	convey.Convey("Given a flat entry plus exit playbook on a sparse tape", t, func() {
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 105,
			},
		}
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 0)
		base := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
			},
		}
		gate := perspectives.Branch{
			Category:    perspectives.CategoryRiskOnSurge,
			Observation: perspectives.ObservationNone,
		}

		convey.Convey("It should allow nesting a gate under entry without requiring exit co-occurrence", func() {
			convey.So(
				nestedEntryGateReachable(index, base, gate),
				convey.ShouldBeTrue,
			)
		})
	})
}

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
