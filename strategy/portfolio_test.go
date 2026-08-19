package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	logicgraph "github.com/theapemachine/symm/logic/graph"
)

func portfolioSummary(score float64) logicgraph.OpportunitySummary {
	return logicgraph.OpportunitySummary{
		Hypothesis:    "hyp:BTC/USD:long_opportunity",
		Support:       score,
		Contradiction: 0,
		Conditions:    0,
		Balance:       1,
		Confidence:    0.8,
		Score:         score,
		Direction:     1,
		Ready:         true,
	}
}

func TestPortfolioStateGetPossibleActions(t *testing.T) {
	Convey("Given one enterable candidate and one open slot", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(0.8),
		}}, 1, 0)

		actions := state.GetPossibleActions()

		Convey("It should offer enter, hold, and done", func() {
			So(actions, ShouldResemble, []float64{
				portfolioEnterReference(0),
				portfolioHoldReference(0),
				portfolioDoneAction,
			})
		})
	})

	Convey("Given zero open slots", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(0.8),
		}}, 0, 0)

		Convey("It should withhold the enter branch and keep the hold path", func() {
			So(state.GetPossibleActions(), ShouldResemble, []float64{
				portfolioHoldReference(0),
				portfolioDoneAction,
			})
		})
	})

	Convey("Given a held leg", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(-0.4), Held: true,
		}}, 0, 0)

		Convey("It should offer exit and hold rather than an entry", func() {
			So(state.GetPossibleActions(), ShouldResemble, []float64{
				portfolioExitReference(0),
				portfolioHoldReference(0),
				portfolioDoneAction,
			})
		})
	})
}

func TestPortfolioStateGetReward(t *testing.T) {
	Convey("Given a flat candidate", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(0.8),
		}}, 1, 0)

		Convey("It should contribute nothing until entered", func() {
			So(state.GetReward(), ShouldEqual, 0)
		})
	})
}

func TestPortfolioSearch(t *testing.T) {
	Convey("Given one strong candidate and one slot", t, func() {
		root, err := portfolioSearch(NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(0.8),
		}}, 1, 0), 32)

		So(err, ShouldBeNil)

		Convey("It should spend the visit budget proving the entry", func() {
			planner := &Planner{}
			choices := planner.portfolioChoices(root, 1)
			So(choices["BTC/USD"], ShouldEqual, portfolioEnterReference(0))
		})
	})

	Convey("Given one held leg whose thesis decayed", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(-0.4), Held: true,
		}}, 0, 0)
		root, err := portfolioSearch(state, 32)

		So(err, ShouldBeNil)

		Convey("It should retire the held lot", func() {
			planner := &Planner{}
			choices := planner.portfolioChoices(root, 1)
			So(choices["BTC/USD"], ShouldEqual, portfolioExitReference(0))
		})
	})
}
