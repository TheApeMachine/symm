package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

func bullishConsensus() *advisor.DeliberationOutcome {
	return &advisor.DeliberationOutcome{
		DominantMove: advisor.MoveExplosivePump,
		Confidence:   0.8,
		Participants: 3,
	}
}

func bearishConsensus() *advisor.DeliberationOutcome {
	return &advisor.DeliberationOutcome{
		DominantMove: advisor.MoveFlashDump,
		Confidence:   0.8,
		Participants: 3,
	}
}

func TestEstimatorAlwaysEstimatesSafeActions(t *testing.T) {
	Convey("Given a bearish consensus and an unarmed precursor", t, func() {
		estimator := &opportunityEstimator{
			consensus:       bearishConsensus(),
			entryAdmissible: false,
		}

		Convey("Wait and Exit remain estimable", func() {
			So(estimator.EstimateAction(nil, mcts.Wait).Defined, ShouldBeTrue)
			So(estimator.EstimateAction(nil, mcts.Exit).Defined, ShouldBeTrue)
		})

		Convey("a held position is never left without a way out", func() {
			exit := estimator.EstimateAction(nil, mcts.Exit)
			So(exit.IdentificationStatus, ShouldEqual, mcts.IdentificationIdentified)
		})
	})
}

func TestEstimatorRefusesEntryOnBearishConsensus(t *testing.T) {
	Convey("Given an armed precursor but a bearish consensus", t, func() {
		estimator := &opportunityEstimator{
			consensus:       bearishConsensus(),
			entryAdmissible: true,
		}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is undefined, not merely penalized", func() {
			So(estimate.Defined, ShouldBeFalse)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationUnsupportedTreatment)
		})
	})
}

func TestEstimatorRefusesEntryWithoutArmedPrecursor(t *testing.T) {
	Convey("Given a bullish consensus but no armed precursor", t, func() {
		estimator := &opportunityEstimator{
			consensus:       bullishConsensus(),
			entryAdmissible: false,
		}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is undefined for want of support", func() {
			So(estimate.Defined, ShouldBeFalse)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationInsufficientSupport)
		})
	})
}

func TestEstimatorAdmitsArmedBullishEntry(t *testing.T) {
	Convey("Given an armed precursor and a bullish consensus", t, func() {
		estimator := &opportunityEstimator{
			consensus:       bullishConsensus(),
			entryAdmissible: true,
		}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is estimable and carries the consensus support", func() {
			So(estimate.Defined, ShouldBeTrue)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationIdentified)
			So(estimate.Support, ShouldEqual, 0.8)
		})
	})
}

func TestPrecursorRuleRejectsIgnition(t *testing.T) {
	Convey("Given the precursor rule", t, func() {
		Convey("an armed long candidate is admissible", func() {
			So(entryAdmissible(types.OpportunityCandidate{
				Phase: types.PhaseArmed, Direction: types.DirectionLong,
			}), ShouldBeTrue)
		})

		Convey("ignition is refused: the move is already visible", func() {
			So(entryAdmissible(types.OpportunityCandidate{
				Phase: types.PhaseIgnition, Direction: types.DirectionLong,
			}), ShouldBeFalse)
		})

		Convey("forming is too early", func() {
			So(entryAdmissible(types.OpportunityCandidate{
				Phase: types.PhaseForming, Direction: types.DirectionLong,
			}), ShouldBeFalse)
		})

		Convey("a short candidate is not entered long", func() {
			So(entryAdmissible(types.OpportunityCandidate{
				Phase: types.PhaseArmed, Direction: types.DirectionShort,
			}), ShouldBeFalse)
		})
	})
}

/*
advisorPerspective builds a resolved perspective for planner deliberation tests.
*/
func advisorPerspective(name, state string, probability float64) *types.Perspective {
	return &types.Perspective{
		Symbol:  "TEST/USD",
		Advisor: name,
		Support: 100,
		Classes: []types.PerspectiveClass{
			{State: types.PerspectiveState(state), Probability: probability},
			{State: "Other", Probability: 1 - probability},
		},
	}
}
