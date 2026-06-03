package beam

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestInsertScoreBeam(t *testing.T) {
	convey.Convey("Given shallow and deep candidates with different scores", t, func() {
		deep := types.CandidateScore{
			Candidate:     1,
			Score:         -0.004,
			AdjustedScore: -0.004,
			ClosedTrades:  1,
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
		flat := types.CandidateScore{
			Candidate:     2,
			Score:         -0.002,
			AdjustedScore: -0.002,
			ClosedTrades:  1,
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

		beam := insertScoreBeam(nil, flat, 4)
		beam = insertScoreBeam(beam, deep, 4)

		convey.Convey("It should retain the higher-scoring playbook regardless of depth", func() {
			convey.So(len(beam), convey.ShouldEqual, 2)
			convey.So(beam[0].Candidate, convey.ShouldEqual, flat.Candidate)
		})
	})
}

func TestCompareBeamCandidatesPrefersScoreOverChurn(t *testing.T) {
	convey.Convey("Given a churny loser and a selective deeper loser", t, func() {
		churn := types.CandidateScore{
			Candidate:     1,
			Score:         -100,
			AdjustedScore: -100,
			ClosedTrades:  10000,
			Branches: perspectives.BranchList{
				{Category: perspectives.CategoryLaminar},
				{Category: perspectives.CategoryExhaustion},
			},
		}
		selective := types.CandidateScore{
			Candidate:     2,
			Score:         -0.05,
			AdjustedScore: -0.05,
			ClosedTrades:  5,
			Branches: perspectives.BranchList{
				{
					Category:    perspectives.CategoryRiskOnSurge,
					Observation: perspectives.ObservationNone,
					Branches: []perspectives.Branch{{
						Category:    perspectives.CategoryLaminar,
						Observation: perspectives.ObservationNotHolding,
					}, {
						Category:    perspectives.CategoryEndogenousAlpha,
						Observation: perspectives.ObservationNone,
						Branches: []perspectives.Branch{{
							Category:    perspectives.CategoryFrenzy,
							Observation: perspectives.ObservationNotHolding,
						}},
					}},
				},
				{Category: perspectives.CategoryExhaustion},
			},
		}

		convey.Convey("It should rank the selective tree ahead of churn", func() {
			convey.So(compareBeamCandidates(selective, churn), convey.ShouldBeTrue)
			convey.So(compareBeamCandidates(churn, selective), convey.ShouldBeFalse)
		})
	})
}

func TestBeamEligibleRejectsInertFlatPairs(t *testing.T) {
	convey.Convey("Given a flat pair with zero closed trades", t, func() {
		entry := types.CandidateScore{
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

func TestTrainSeedEligibleKeepsDeepPlaybooks(t *testing.T) {
	convey.Convey("Given a deep unprofitable playbook", t, func() {
		entry := types.CandidateScore{
			AdjustedScore: -0.01,
			ClosedTrades:  0,
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

		convey.Convey("It should remain eligible as an MCTS seed", func() {
			convey.So(trainSeedEligible(entry), convey.ShouldBeTrue)
		})
	})
}
