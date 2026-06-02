package optimizer

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestInsertDepthStratifiedBeam(t *testing.T) {
	convey.Convey("Given shallow candidates that score slightly better than deep ones", t, func() {
		deep := CandidateScore{
			Candidate:    1,
			Score:        -0.004,
			ClosedTrades: 1,
			Branches: perspectives.BranchList{
				{
					Category:    perspectives.CategoryRiskOnSurge,
					Observation: perspectives.ObservationNone,
					Branches: []perspectives.Branch{{
						Category:    perspectives.CategoryLaminar,
						Observation: perspectives.ObservationNotHolding,
					}},
				},
			},
		}
		flat := CandidateScore{
			Candidate:    2,
			Score:        -0.002,
			ClosedTrades: 1,
			Branches: perspectives.BranchList{
				{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
				},
				{
					Category:    perspectives.CategoryExhaustion,
					Observation: perspectives.ObservationHolding,
				},
			},
		}

		beam := insertDepthStratifiedBeam(nil, flat, 4)
		beam = insertDepthStratifiedBeam(beam, deep, 4)

		convey.Convey("It should retain the deeper playbook over shallow traders", func() {
			maxDepth := 0

			for _, candidate := range beam {
				depth := reasoningDepth(candidate.Branches)

				if depth > maxDepth {
					maxDepth = depth
				}
			}

			convey.So(maxDepth, convey.ShouldBeGreaterThan, 1)
		})
	})
}

func TestRecordBeamAmortizesPruning(t *testing.T) {
	convey.Convey("Given a scan search with a small beam width", t, func() {
		search := &ScanSearch{options: ScanOptions{BeamWidth: 1}}

		for candidate := 1; candidate <= 5; candidate++ {
			search.recordBeam(CandidateScore{
				Candidate:    candidate,
				Score:        float64(candidate) * -0.001,
				ClosedTrades: 1,
				Branches: perspectives.BranchList{
					{Category: perspectives.CategoryLaminar},
				},
			})
		}

		convey.Convey("It should prune once the buffer exceeds four times the beam width", func() {
			convey.So(len(search.beamScores), convey.ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestBeamEligibleRejectsInertFlatPairs(t *testing.T) {
	convey.Convey("Given a flat pair with zero closed trades", t, func() {
		entry := CandidateScore{
			ClosedTrades: 0,
			Branches: perspectives.BranchList{
				{Category: perspectives.CategoryLaminar},
				{Category: perspectives.CategoryExhaustion},
			},
		}

		convey.Convey("It should not be beam eligible", func() {
			convey.So(beamEligible(entry), convey.ShouldBeFalse)
		})
	})
}
