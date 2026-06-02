package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestPrecompileTapeRingWindow(t *testing.T) {
	convey.Convey("Given more than StoryRingCapacity measurements", t, func() {
		rows := make([]perspectives.Measurement, 0, StoryRingCapacity+10)

		for index := range StoryRingCapacity + 10 {
			rows = append(rows, perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      float64(index),
				Last:     100 + float64(index),
			})
		}

		tape := PrecompileTape(rows)
		lastTick := tape.Ticks[len(rows)-1]

		convey.Convey("It should cap the decision snapshot to the story ring size", func() {
			convey.So(len(lastTick.Snapshots), convey.ShouldEqual, StoryRingCapacity)
			convey.So(lastTick.Snapshots[0].SNR, convey.ShouldEqual, 10)
		})
	})
}

func TestScanSearchDeepensEachBeamPass(t *testing.T) {
	convey.Convey("Given co-occurring categories across a replay tape", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
				Category: perspectives.CategoryEndogenousAlpha, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceHawkes,
				Category: perspectives.CategoryFrenzy, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceCVD,
				Category: perspectives.CategoryAggressiveDrive, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryActiveReversal, SNR: 2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryActiveReversal, SNR: 2, Last: 105,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		scored := make([]CandidateScore, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         32,
			CandidateLimit:    4096,
			MaxReasoningSteps: 6,
		})
		search.onCandidate = func(candidate CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should score trees deeper than two reasoning steps", func() {
			deepest := 0

			for _, candidate := range scored {
				depth := reasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThan, 2)
		})
	})
}

func TestTreeSearchApplyMoveNestsGates(t *testing.T) {
	convey.Convey("Given a flat entry playbook", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
		})
		profile.Add(perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
			Category: perspectives.CategoryRiskOnSurge, SNR: 2, Last: 100,
		})
		profile.PrepareCache()

		search := NewHybridTreeSearch(ctx, &profile, profile.Rows(), GuardOptions{}, nil, MCTSOptions{
			MaxReasoningSteps: 8,
		})
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
			quantile:    0.5,
			action:      perspectives.ActionNone,
		}

		nested := search.applyMove(entry, gateMove)

		convey.Convey("It should nest deny gates under the entry chain", func() {
			convey.So(reasoningDepth(nested), convey.ShouldEqual, 2)
			convey.So(nested[0].Category, convey.ShouldEqual, perspectives.CategoryRiskOnSurge)
			convey.So(nested[0].Branches[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
		})
	})
}
