package scan_test

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/scan"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestScanSearchEmitsNestedEntryPlaybooks(t *testing.T) {
	convey.Convey("Given replay rows and a multi-depth scan budget", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
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
				SNR:      2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 105,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		scored := make([]types.CandidateScore, 0)
		search := scan.NewScanSearch(ctx, profile, rows, types.ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         16,
			CandidateLimit:    4096,
			MaxReasoningSteps: 4,
		})
		search.OnCandidate = func(candidate types.CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should score playbooks deeper than flat entry plus exit", func() {
			convey.So(len(scored), convey.ShouldBeGreaterThan, 0)

			deepest := 0

			for _, candidate := range scored {
				depth := playbook.ReasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThanOrEqualTo, 2)
		})
	})
}

func TestScanSearchProgressesBeyondDepthTwo(t *testing.T) {
	convey.Convey("Given alternating measurements across many categories", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		rows := make([]perspectives.Measurement, 0, 240)

		for tick := 0; tick < 120; tick++ {
			price := 100.0

			if tick%2 == 1 {
				price = 110
			}

			rows = append(rows,
				perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
					Category: perspectives.CategoryLaminar,
					SNR:      2, Last: price,
				},
				perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
					Category: perspectives.CategoryRiskOnSurge,
					SNR:      2, Last: price,
				},
				perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
					Category: perspectives.CategoryExhaustion,
					SNR:      2, Last: price,
				},
				perspectives.Measurement{
					Symbol: "BTC/EUR", Source: perspectives.SourceCausal,
					Category: perspectives.CategoryEndogenousAlpha,
					SNR:      2, Last: price,
				},
			)
		}

		for _, row := range rows {
			profile.Add(row)
		}

		scored := make([]types.CandidateScore, 0)
		search := scan.NewScanSearch(ctx, profile, rows, types.ScanOptions{
			Workers:           2,
			MaxThresholds:     8,
			BeamWidth:         32,
			CandidateLimit:    4096,
			MaxReasoningSteps: 4,
		})
		search.OnCandidate = func(candidate types.CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should reach reasoning depth beyond two", func() {
			entry := perspectives.Branch{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			}
			exit := perspectives.Branch{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			}
			gate := perspectives.Branch{
				Category:    perspectives.CategoryRiskOnSurge,
				Observation: perspectives.ObservationNone,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1,
				ValueSet:    true,
			}
			nested, ok := playbook.NestGateUnderEntry(perspectives.BranchList{entry, exit}, gate)
			baselineDepth := playbook.ReasoningDepth(nested)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(scored), convey.ShouldBeGreaterThan, 0)

			deepest := 0

			for _, candidate := range scored {
				depth := playbook.ReasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThan, baselineDepth)
		})
	})
}

func TestScanSearchDeepensEachBeamPass(t *testing.T) {
	convey.Convey("Given co-occurring categories across a replay tape", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
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

		scored := make([]types.CandidateScore, 0)
		search := scan.NewScanSearch(ctx, profile, rows, types.ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         32,
			CandidateLimit:    4096,
			MaxReasoningSteps: 6,
		})
		search.OnCandidate = func(candidate types.CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should score trees deeper than two reasoning steps", func() {
			deepest := 0

			for _, candidate := range scored {
				depth := playbook.ReasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThan, 2)
		})
	})
}

func TestScanSearchIgnoresInertZeroReturn(t *testing.T) {
	convey.Convey("Given an inert candidate before active losers", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 95,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		bestScores := make([]float64, 0)
		search := scan.NewScanSearch(ctx, profile, rows, types.ScanOptions{
			Workers:           1,
			MaxThresholds:     2,
			BeamWidth:         4,
			CandidateLimit:    64,
			MaxReasoningSteps: 1,
		})
		search.OnBest = func(best types.BestTree) {
			bestScores = append(bestScores, best.Score)
		}
		search.Run()

		convey.Convey("It should not lock YAML to an inert 0% return", func() {
			for _, score := range bestScores {
				convey.So(score, convey.ShouldNotEqual, 0)
			}
		})
	})
}

func TestScanSearchOnBestTracksBestCandidate(t *testing.T) {
	convey.Convey("Given a losing replay tape", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 95,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		bestCount := 0
		search := scan.NewScanSearch(ctx, profile, rows, types.ScanOptions{
			Workers:           2,
			MaxThresholds:     2,
			BeamWidth:         4,
			CandidateLimit:    64,
			MaxReasoningSteps: 2,
		})
		search.OnBest = func(best types.BestTree) {
			bestCount++
		}
		search.Run()

		convey.Convey("It should persist the best losing tree", func() {
			convey.So(bestCount, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestBuildDecisionSeedPlaybooks(t *testing.T) {
	convey.Convey("Given a tape with a DECISION.md entry chain", t, func() {
		profile := profile.NewProfile(context.Background())
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
		index := cooccurrence.NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)
		playbooks := budget.BuildDecisionSeedPlaybooks(profile, index)

		convey.Convey("It should emit nested reachable decision seeds", func() {
			convey.So(len(playbooks), convey.ShouldBeGreaterThan, 0)

			deepest := 0

			for _, branches := range playbooks {
				depth := playbook.ReasoningDepth(branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThanOrEqualTo, 3)
		})
	})
}
