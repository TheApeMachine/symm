package cooccurrence

import (
	"context"
	"sync"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
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
		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)

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

func TestEntryExitPairReachableWithoutCoOccurrence(t *testing.T) {
	convey.Convey("Given entry and exit categories that never share a tick", t, func() {
		rows := make([]perspectives.Measurement, 0, replay.StoryRingCapacity+2)
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2, Last: 100,
		})

		for range replay.StoryRingCapacity {
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

		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)
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
		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)
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
		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 4)
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

func TestMovesApply(t *testing.T) {
	convey.Convey("Given a near-miss nested gate", t, func() {
		profile := profile.NewProfile(context.Background())
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

		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 4)
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

func TestCoOccurrenceIndexChainReachabilityScoreConcurrent(t *testing.T) {
	convey.Convey("Given parallel MCTS workers sharing one co-occurrence index", t, func() {
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
		index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)
		chains := [][]perspectives.CategoryType{
			{perspectives.CategoryLaminar, perspectives.CategoryExhaustion},
			{perspectives.CategoryLaminar, perspectives.CategoryToxicBluff},
			{perspectives.CategoryExhaustion, perspectives.CategoryTurbulent},
		}
		var workers sync.WaitGroup

		for worker := range 16 {
			workers.Add(1)

			go func() {
				defer workers.Done()

				for range 256 {
					chain := chains[worker%len(chains)]
					_ = index.ChainReachabilityScore(chain)
				}
			}()
		}

		workers.Wait()

		convey.Convey("It should cache reachability without data races", func() {
			convey.So(len(index.reachabilityCache), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func init() {
	if viper.GetViper().GetFloat64("signals.causal.condition_switch") <= 0 {
		viper.Set("signals.causal.condition_switch", 100.0)
	}
}
