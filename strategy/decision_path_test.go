package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

/*
coiledCouncil is the archetype the system exists to catch: a liquidity sweep
followed by a bid wall, with shorts piling in. The three readings are mutually
reinforcing, so the War Room's synergy rules should fire.
*/
func coiledCouncil() *advisor.DeliberationOutcome {
	room := advisor.NewWarRoom()

	// Advisors speak on a trade envelope when a volume bar completes.
	room.Deliberate([]*types.Perspective{
		advisorPerspective("pullback", "LiquiditySweep", 0.80),
		advisorPerspective("liquidity", "WallBuilding", 0.85),
		advisorPerspective("basis", "LeverageSqueeze", 0.75),
	}, "TEST/USD", time.Unix(1, 0))

	// The planner then decides on a ticker envelope carrying no perspectives.
	return room.Deliberate(nil, "TEST/USD", time.Unix(2, 0))
}

/*
plannerSearch builds the search exactly as NewPlanner configures it, so these
tests exercise the shipped policy rather than a convenient one.
*/
func plannerSearch(seed int64) *mcts.Search {
	search := mcts.NewSearch(
		searchIterations, searchExploration, searchUncertainty, seed,
	)
	search.Causal = mcts.DefaultCausalEngine{Linear: true}
	search.CausalPolicy = mcts.EconomicCausalPolicy(
		causalMinimumRows,
		causalExpectationWeight,
		causalMaxCounterfactualMass,
		true,
	).WithRejectionFloor(causalRejectionMargin)

	return search
}

func sampleObservationalHistory(count int) [][]float64 {
	rows := make([][]float64, count)

	for index := 0; index < count; index++ {
		exposure := 0.0
		wealthChange := 0.0

		if index%2 == 1 {
			exposure = 20.0
			wealthChange = 0.5 + float64(index)*0.05
		}

		rows[index] = []float64{
			100.0 + float64(index)*0.1,
			float64(index % searchHorizon),
			exposure,
			wealthChange,
		}
	}

	return rows
}

func TestCoiledSetupSelectsEntryEndToEnd(t *testing.T) {
	Convey("Given a coiled council and no calibrated resonance", t, func() {
		consensus := coiledCouncil()

		Convey("the resident council survives the envelope boundary", func() {
			So(consensus.Participants, ShouldEqual, 3)
			So(consensus.Synergies, ShouldNotBeEmpty)
			So(consensus.DominantMove, ShouldEqual, advisor.MoveExplosivePump)
		})

		model, ready := newConsensusMarketModel(consensus, tickerCadence)
		So(ready, ShouldBeTrue)

		state := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			mcts.MarketState{At: time.Unix(2, 0)},
			model,
			mcts.CostModel{FeeRate: 0.0026, SpreadFraction: 0.0005},
			20, 20, searchHorizon,
		).WithHistory(sampleObservationalHistory(16))

		result := plannerSearch(7).Run(state, &consensusEstimator{
			consensus: consensus,
		})

		Convey("the causal search selects entry", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.SelectedAction, ShouldEqual, mcts.Enter)
			So(result.IdentificationStatus, ShouldEqual, mcts.IdentificationIdentified)
		})

		Convey("entry beats waiting on the blended value", func() {
			var enter, wait *mcts.BranchTrace

			for index := range result.Trace.Branches {
				switch result.Trace.Branches[index].Action {
				case mcts.Enter:
					enter = &result.Trace.Branches[index]
				case mcts.Wait:
					wait = &result.Trace.Branches[index]
				}
			}

			So(enter, ShouldNotBeNil)
			So(wait, ShouldNotBeNil)
			So(enter.BlendedValue, ShouldBeGreaterThan, wait.BlendedValue)
		})

		Convey("the causal layer actually engaged", func() {
			counterfactual := 0.0
			identified := 0

			for _, branch := range result.Trace.Branches {
				counterfactual += branch.CounterfactualMass

				if branch.CausalExpectationDefined {
					identified++
				}
			}

			So(counterfactual, ShouldBeGreaterThan, 0)
			So(identified, ShouldBeGreaterThan, 0)
		})

		Convey("the dominated branch is pruned under the shipped policy", func() {
			pruned := 0

			for _, branch := range result.Trace.Branches {
				if branch.Pruned {
					pruned++
				}
			}

			So(pruned, ShouldBeGreaterThan, 0)
		})
	})
}

func TestBearishCouncilRefusesEntryEndToEnd(t *testing.T) {
	Convey("Given a council reading a bull trap", t, func() {
		room := advisor.NewWarRoom()
		room.Deliberate([]*types.Perspective{
			advisorPerspective("momentum", "Building", 0.80),
			advisorPerspective("auction", "SellersAbsorbing", 0.92),
			advisorPerspective("liquidity", "VacuumForming", 0.85),
		}, "TEST/USD", time.Unix(1, 0))

		consensus := room.Deliberate(nil, "TEST/USD", time.Unix(2, 0))

		Convey("the veto is recorded and the consensus turns bearish", func() {
			So(consensus.Vetoes, ShouldNotBeEmpty)
			So(consensus.DominantMove, ShouldBeLessThan, advisor.MoveWeakDrift)
		})

		model, ready := newConsensusMarketModel(consensus, tickerCadence)
		So(ready, ShouldBeTrue)

		Convey("the modeled drift is negative", func() {
			So(model.drift, ShouldBeLessThan, 0)
		})

		state := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			mcts.MarketState{At: time.Unix(2, 0)},
			model,
			mcts.CostModel{FeeRate: 0.0026, SpreadFraction: 0.0005},
			20, 20, searchHorizon,
		)

		result := plannerSearch(11).Run(state, &consensusEstimator{
			consensus: consensus,
		})

		Convey("entry is never selected into a vetoed book", func() {
			So(result.SelectedAction, ShouldNotEqual, mcts.Enter)
		})
	})
}

func TestConsensusDriftClearsRoundTripFriction(t *testing.T) {
	Convey("Given a strongly bullish council", t, func() {
		consensus := coiledCouncil()
		model, ready := newConsensusMarketModel(consensus, tickerCadence)
		So(ready, ShouldBeTrue)

		Convey("the expected move over the horizon exceeds round-trip cost", func() {
			// Taker fee both sides plus spread is roughly 0.6%. A regime model
			// whose expected move cannot clear that will always lose to Wait,
			// which is what made the bounded resonance drift unusable on a
			// cold start.
			expected := model.drift * float64(searchHorizon)
			roundTrip := 2*0.0026 + 0.0005

			So(expected, ShouldBeGreaterThan, roundTrip)
		})

		Convey("the rollout stays stochastic", func() {
			So(model.volatility, ShouldBeGreaterThan, 0)
		})
	})
}

func TestConsensusModelSignsWithTheCouncil(t *testing.T) {
	Convey("Given councils reading opposite regimes", t, func() {
		bullish, bullOK := newConsensusMarketModel(coiledCouncil(), tickerCadence)
		So(bullOK, ShouldBeTrue)

		room := advisor.NewWarRoom()
		room.Deliberate([]*types.Perspective{
			advisorPerspective("liquidity", "VacuumForming", 0.90),
		}, "TEST/USD", time.Unix(1, 0))

		bearish, bearOK := newConsensusMarketModel(
			room.Deliberate(nil, "TEST/USD", time.Unix(2, 0)), tickerCadence,
		)
		So(bearOK, ShouldBeTrue)

		Convey("drift follows the council's direction", func() {
			So(bullish.drift, ShouldBeGreaterThan, 0)
			So(bearish.drift, ShouldBeLessThan, 0)
		})
	})

	Convey("Given no council at all", t, func() {
		_, ready := newConsensusMarketModel(nil, tickerCadence)

		Convey("no model is produced", func() {
			So(ready, ShouldBeFalse)
		})
	})
}

func TestDecisionCarriesItsReasoningTrace(t *testing.T) {
	Convey("Given a coiled council driving a search", t, func() {
		consensus := coiledCouncil()
		model, ready := newConsensusMarketModel(consensus, tickerCadence)
		So(ready, ShouldBeTrue)

		state := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			mcts.MarketState{At: time.Unix(2, 0)},
			model,
			mcts.CostModel{FeeRate: 0.0026, SpreadFraction: 0.0005},
			20, 20, searchHorizon,
		).WithHistory(sampleObservationalHistory(16))

		result := plannerSearch(7).Run(state, &consensusEstimator{
			consensus: consensus,
		})

		trace := buildTrace(consensus, result, "war-room-consensus")

		Convey("the council's deliberation is recorded", func() {
			So(trace.Deliberation.Participants, ShouldEqual, 3)
			So(trace.Deliberation.DominantMove, ShouldEqual, "explosive_pump")
			So(trace.Deliberation.Synergies, ShouldNotBeEmpty)

			Convey("including the full move distribution", func() {
				So(trace.Deliberation.Probabilities, ShouldContainKey, "explosive_pump")
				So(trace.Deliberation.Probabilities, ShouldContainKey, "flash_dump")
			})
		})

		Convey("the search configuration and verdict are recorded", func() {
			So(trace.MCTS.Iterations, ShouldEqual, searchIterations)
			So(trace.MCTS.Horizon, ShouldEqual, searchHorizon)
			So(trace.MCTS.RecommendedAction, ShouldEqual, "enter")
			So(trace.MCTS.TransitionSource, ShouldEqual, "war-room-consensus")
			So(trace.MCTS.IdentificationStatus, ShouldEqual, "identified")
		})

		Convey("the explored tree is recorded with real depth", func() {
			So(trace.MCTS.Tree, ShouldNotBeNil)
			So(trace.MCTS.Tree.Action, ShouldEqual, "root")
			So(trace.MCTS.Tree.Children, ShouldNotBeEmpty)
			So(trace.MCTS.MaxDepth, ShouldBeGreaterThan, 1)
			So(trace.MCTS.TotalNodes, ShouldBeGreaterThan, 2)
		})

		Convey("the selected branch is marked on the tree", func() {
			marked := 0

			for _, child := range trace.MCTS.Tree.Children {
				if child.Selected {
					marked++
					So(child.Action, ShouldEqual, "enter")
				}
			}

			So(marked, ShouldEqual, 1)
		})

		Convey("real and virtual evidence stay distinguishable", func() {
			counterfactual := 0.0

			for _, branch := range trace.MCTS.Branches {
				counterfactual += branch.CounterfactualMass

				// The blended value is derivable from the parts, so an
				// operator can see how much of it was imagined.
				So(branch.EffectiveVisits, ShouldAlmostEqual,
					float64(branch.Visits)+branch.CounterfactualMass, 0.000001)
			}

			So(counterfactual, ShouldBeGreaterThan, 0)
		})
	})
}

func TestTraceEncodesToTelemetry(t *testing.T) {
	Convey("Given a populated decision trace", t, func() {
		consensus := coiledCouncil()
		model, _ := newConsensusMarketModel(consensus, tickerCadence)

		state := mcts.NewEconomicState(
			mcts.PortfolioState{Cash: 10000, Position: 0, MarkPrice: 100},
			mcts.MarketState{At: time.Unix(2, 0)},
			model,
			mcts.CostModel{FeeRate: 0.0026},
			20, 20, searchHorizon,
		).WithHistory(sampleObservationalHistory(16))

		result := plannerSearch(7).Run(state, &consensusEstimator{
			consensus: consensus,
		})

		decision := types.NewDecision(types.ActionEnter, "TEST/USD")
		decision.Trace = buildTrace(consensus, result, "war-room-consensus")

		wire := types.DecisionWire(decision)

		Convey("the wire decision carries the trace", func() {
			So(wire.Trace, ShouldNotBeNil)
			So(wire.Trace.RecommendedAction, ShouldEqual, "enter")
			So(wire.Trace.ConsensusParticipants, ShouldEqual, 3)
			So(wire.Trace.Tree, ShouldNotBeNil)
			So(wire.Trace.Tree.Children, ShouldNotBeEmpty)
			So(wire.Trace.Branches, ShouldNotBeEmpty)
		})
	})
}
