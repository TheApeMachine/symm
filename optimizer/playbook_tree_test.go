package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNestGateUnderEntry(t *testing.T) {
	convey.Convey("Given a flat entry and exit playbook", t, func() {
		entry := perspectives.Branch{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}
		exit := perspectives.Branch{
			Category:    perspectives.CategoryExhaustion,
			Observation: perspectives.ObservationHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
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
		playbook := perspectives.BranchList{entry, exit}

		nested, ok := nestGateUnderEntry(playbook, gate)

		convey.Convey("It should nest the gate sequentially under the entry root", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(nested), convey.ShouldEqual, 2)
			convey.So(nested[0].Category, convey.ShouldEqual, perspectives.CategoryRiskOnSurge)
			convey.So(len(nested[0].Branches), convey.ShouldEqual, 1)
			convey.So(nested[0].Branches[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
			convey.So(reasoningDepth(nested), convey.ShouldEqual, 2)
		})
	})
}

func TestWidenWithExit(t *testing.T) {
	convey.Convey("Given a playbook with an exit sibling", t, func() {
		playbook := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Action:      perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Action:      perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		alternateExit := perspectives.Branch{
			Category:    perspectives.CategoryActiveReversal,
			Observation: perspectives.ObservationHolding,
			Action:      perspectives.Action{Type: perspectives.ActionStopLossLimit},
		}

		widened, ok := widenWithExit(playbook, alternateExit)

		convey.Convey("It should swap the exit branch without changing depth", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(reasoningDepth(widened), convey.ShouldEqual, reasoningDepth(playbook))
			convey.So(widened[1].Category, convey.ShouldEqual, perspectives.CategoryActiveReversal)
		})
	})
}

func TestScanSearchEmitsNestedEntryPlaybooks(t *testing.T) {
	convey.Convey("Given replay rows and a multi-depth scan budget", t, func() {
		ctx := context.Background()
		profile := Profile{ctx: ctx}
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

		scored := make([]CandidateScore, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     4,
			BeamWidth:         16,
			CandidateLimit:    4096,
			MaxReasoningSteps: 4,
		})
		search.onCandidate = func(candidate CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should score playbooks deeper than flat entry plus exit", func() {
			convey.So(len(scored), convey.ShouldBeGreaterThan, 0)

			deepest := 0

			for _, candidate := range scored {
				depth := reasoningDepth(candidate.Branches)

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
		profile := Profile{ctx: ctx}
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

		scored := make([]CandidateScore, 0)
		search := NewScanSearch(ctx, &profile, rows, ScanOptions{
			Workers:           2,
			MaxThresholds:     8,
			BeamWidth:         32,
			CandidateLimit:    4096,
			MaxReasoningSteps: 4,
		})
		search.onCandidate = func(candidate CandidateScore) {
			scored = append(scored, candidate)
		}
		search.Run()

		convey.Convey("It should reach reasoning depth beyond two", func() {
			deepest := 0

			for _, candidate := range scored {
				depth := reasoningDepth(candidate.Branches)

				if depth > deepest {
					deepest = depth
				}
			}

			convey.So(deepest, convey.ShouldBeGreaterThanOrEqualTo, 3)
		})
	})
}
