package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestPairAffinityRanksExits(t *testing.T) {
	convey.Convey("Given recorded flat pair scores", t, func() {
		index := NewPairAffinityIndex()
		index.RecordFlatPair(
			perspectives.CategoryLaminar,
			perspectives.CategoryExhaustion,
			-0.02,
		)
		index.RecordFlatPair(
			perspectives.CategoryLaminar,
			perspectives.CategoryActiveReversal,
			-1.66,
		)

		exits := []scanCandidate{
			{branches: perspectives.BranchList{{
				Category: perspectives.CategoryActiveReversal,
			}}},
			{branches: perspectives.BranchList{{
				Category: perspectives.CategoryExhaustion,
			}}},
		}

		ranked := rankExitsByAffinity(
			index, perspectives.CategoryLaminar, exits,
		)

		convey.Convey("It should prefer the less catastrophic exit first", func() {
			convey.So(
				exitCategory(ranked[0]),
				convey.ShouldEqual,
				perspectives.CategoryExhaustion,
			)
		})
	})
}

func TestScanSearchDeepensWithDiverseSurvivors(t *testing.T) {
	convey.Convey("Given beam survivors with different entry categories", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceHawkes,
				Category: perspectives.CategoryFrenzy, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 105,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		scored := make([]CandidateScore, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         8,
			CandidateLimit:    2048,
			MaxReasoningSteps: 4,
		})
		search.onCandidate = func(candidate CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should still score nested trees", func() {
			deepest := 0

			for _, candidate := range scored {
				depth := reasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThan, 1)
		})
	})
}
