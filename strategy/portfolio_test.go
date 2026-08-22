package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
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

	Convey("Given a classified opportunity whose summary carries a different score", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol:  "PUMP/USD",
			Summary: portfolioSummary(0.8),
			Opportunity: logicgraph.OpportunityScore{
				Type:      types.OpportunitySuddenPump,
				Lifecycle: types.LifecycleEmergent,
				Score:     0.2,
			},
		}}, 1, 0)
		postState := state.ApplyAction(portfolioEnterReference(0))

		Convey("It should use the already trust-weighted classification exactly once", func() {
			So(postState.GetReward(), ShouldEqual, 0.2)
		})
	})

	Convey("Given structural evidence with no classified opportunity", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol:  "STRUCTURAL/USD",
			Summary: portfolioSummary(0.8),
			Opportunity: logicgraph.OpportunityScore{
				Type: types.OpportunityNone,
			},
		}}, 1, 0)
		postState := state.ApplyAction(portfolioEnterReference(0))

		Convey("It should fall back to the structural summary without attenuation", func() {
			So(postState.GetReward(), ShouldEqual, 0.8)
		})
	})
}

func TestPortfolioStateIsTerminal(t *testing.T) {
	Convey("Given a free slot and a rollout that reaches its declared horizon", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol: "BTC/USD", Summary: portfolioSummary(0.8),
		}}, 1, 0)
		current := state.ApplyAction(portfolioHoldReference(0)).(*PortfolioState)
		current = current.ApplyAction(portfolioHoldReference(0)).(*PortfolioState)

		Convey("It should stop even though capacity remains unused", func() {
			So(current.IsTerminal(), ShouldBeTrue)
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

	Convey("Given a weak but positively classified opportunity and a free slot", t, func() {
		state := NewPortfolioState([]portfolioLeg{{
			Symbol:  "ENA/USD",
			Summary: portfolioSummary(0.8),
			Opportunity: logicgraph.OpportunityScore{
				Type:      types.OpportunitySuddenPump,
				Lifecycle: types.LifecycleEmergent,
				Score:     0.02,
			},
		}}, 1, 0)
		root, err := portfolioSearch(state, 16)

		So(err, ShouldBeNil)

		Convey("It should select the positive entry instead of resolving an all-zero tie", func() {
			planner := &Planner{}
			So(planner.portfolioChoices(root, 1)["ENA/USD"],
				ShouldEqual, portfolioEnterReference(0))
		})
	})

	Convey("Given the same portfolio state and search budget repeatedly", t, func() {
		state := NewPortfolioState([]portfolioLeg{
			{Symbol: "ENA/USD", Summary: portfolioSummary(0.4)},
			{Symbol: "DOGE/USD", Summary: portfolioSummary(0.3)},
		}, 1, 0)
		var expected []struct {
			action float64
			visits int
			reward float64
		}

		for run := 0; run < 16; run++ {
			root, err := portfolioSearch(state, 32)
			So(err, ShouldBeNil)
			observed := make([]struct {
				action float64
				visits int
				reward float64
			}, len(root.Children))

			for index, child := range root.Children {
				observed[index].action = child.Action
				observed[index].visits = child.Visits
				observed[index].reward = child.TotalReward
			}

			if run == 0 {
				expected = observed
				continue
			}

			So(observed, ShouldResemble, expected)
		}
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

	Convey("Given a held maturing leg with a valid thesis and a marginally higher flat candidate", t, func() {
		state := NewPortfolioState([]portfolioLeg{
			{
				Symbol:            "BTC/USD",
				Summary:           portfolioSummary(0.05),
				Held:              true,
				Maturing:          true,
				Observed:          2,
				Horizon:           20,
				SwitchingCost:     0.016,
				ContinuationValue: 0.05,
			},
			{
				Symbol:  "SOL/USD",
				Summary: portfolioSummary(0.06),
			},
		}, 0, 0)
		root, err := portfolioSearch(state, 64)

		So(err, ShouldBeNil)

		Convey("It should hold the maturing lot rather than churning into the marginal candidate", func() {
			planner := &Planner{}
			choices := planner.portfolioChoices(root, 2)
			So(choices["BTC/USD"], ShouldEqual, portfolioHoldReference(0))
		})
	})

	Convey("Given a held maturing leg when a dominant breakout candidate appears", t, func() {
		state := NewPortfolioState([]portfolioLeg{
			{
				Symbol:            "BTC/USD",
				Summary:           portfolioSummary(0.02),
				Held:              true,
				Maturing:          true,
				Observed:          2,
				Horizon:           20,
				SwitchingCost:     0.016,
				ContinuationValue: 0.02,
			},
			{
				Symbol:  "SOL/USD",
				Summary: portfolioSummary(0.8),
			},
		}, 0, 0)
		root, err := portfolioSearch(state, 64)

		So(err, ShouldBeNil)

		Convey("It should rotate because the alternative expected value overwhelms the switching hurdle", func() {
			planner := &Planner{}
			choices := planner.portfolioChoices(root, 2)
			So(choices["BTC/USD"], ShouldEqual, portfolioExitReference(0))
		})
	})
}

func BenchmarkPortfolioSearch(b *testing.B) {
	state := NewPortfolioState([]portfolioLeg{
		{Symbol: "ENA/USD", Summary: portfolioSummary(0.4)},
		{Symbol: "DOGE/USD", Summary: portfolioSummary(0.3)},
	}, 1, 0)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := portfolioSearch(state, 32); err != nil {
			b.Fatal(err)
		}
	}
}
