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
	Convey("Given a bearish consensus", t, func() {
		estimator := &consensusEstimator{
			consensus: bearishConsensus(),
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
	Convey("Given a bearish consensus", t, func() {
		estimator := &consensusEstimator{
			consensus: bearishConsensus(),
		}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is undefined, not merely penalized", func() {
			So(estimate.Defined, ShouldBeFalse)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationUnsupportedTreatment)
		})
	})
}

func TestEstimatorRefusesEntryWithoutConsensus(t *testing.T) {
	Convey("Given no council consensus at all", t, func() {
		estimator := &consensusEstimator{}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is not identifiable", func() {
			So(estimate.Defined, ShouldBeFalse)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationNotIdentifiable)
		})

		Convey("the safe actions still resolve", func() {
			So(estimator.EstimateAction(nil, mcts.Wait).Defined, ShouldBeTrue)
			So(estimator.EstimateAction(nil, mcts.Exit).Defined, ShouldBeTrue)
		})
	})
}

func TestEstimatorAdmitsBullishEntry(t *testing.T) {
	Convey("Given a bullish consensus", t, func() {
		estimator := &consensusEstimator{
			consensus: bullishConsensus(),
		}

		estimate := estimator.EstimateAction(nil, mcts.Enter)

		Convey("entry is estimable and carries the consensus support", func() {
			So(estimate.Defined, ShouldBeTrue)
			So(estimate.IdentificationStatus, ShouldEqual, mcts.IdentificationIdentified)
			So(estimate.Support, ShouldEqual, 0.8)
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
