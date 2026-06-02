package optimizer

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
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
