package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/types"
)

/*
tickerEnvelope builds a well-formed ticker envelope for planner tests.
*/
func tickerEnvelope(symbol string, bid float64, ask float64) *types.Envelope {
	return &types.Envelope{
		TypeID: types.EnvelopeTicker,
		TickerData: kraken.TickerData{
			Symbol:    symbol,
			Bid:       decimal.NewFromFloat64(bid),
			Ask:       decimal.NewFromFloat64(ask),
			Timestamp: time.Unix(1, 0),
		},
	}
}

/*
plannerForTest builds a planner with a live War Room but no desk, so plan can
be exercised without a broker.
*/
func plannerForTest() *Planner {
	return &Planner{warRoom: advisor.NewWarRoom()}
}

func TestPlannerRejectsMalformedTicker(t *testing.T) {
	Convey("Given a ticker missing its event time", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.TickerData.Timestamp = time.Time{}

		Convey("the planner halts rather than deciding on an invalid frame", func() {
			So(planner.Step(envelope), ShouldBeNil)
			So(planner.Error(), ShouldNotBeNil)

			Convey("and it stays halted for every later frame", func() {
				So(planner.Step(tickerEnvelope("TEST/USD", 100, 101)), ShouldBeNil)
			})
		})
	})

	Convey("Given a ticker with a non-positive bid", t, func() {
		planner := plannerForTest()

		Convey("the planner halts", func() {
			So(planner.Step(tickerEnvelope("TEST/USD", 0, 101)), ShouldBeNil)
			So(planner.Error(), ShouldNotBeNil)
		})
	})
}

func TestPlannerRequiresAdvisorsBeforeDeciding(t *testing.T) {
	Convey("Given a ticker with no advisor perspectives", t, func() {
		planner := plannerForTest()

		out := planner.Step(tickerEnvelope("TEST/USD", 100, 101))

		Convey("no entry is admitted and the reason names the silence", func() {
			So(out, ShouldNotBeNil)
			So(out.StrategyRound, ShouldNotBeNil)

			decision := out.StrategyRound.Decisions[0]
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.PredictiveStatus, ShouldEqual, "awaiting-advisor-consensus")
		})
	})
}

func TestPlannerFallsBackToConsensusWhenResonanceIsCold(t *testing.T) {
	Convey("Given advisors but no calibrated resonance forecast", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
		}

		out := planner.Step(envelope)
		decision := out.StrategyRound.Decisions[0]

		Convey("planning proceeds on the council's own distribution", func() {
			// A cold start is exactly when a pump is most likely and least
			// modeled; aborting here would blind the system to its best setups.
			So(decision.PredictiveStatus, ShouldNotEqual, "no-transition-model")
			So(decision.ForecastSource, ShouldEqual, "war-room-consensus")
		})
	})

	Convey("Given no advisors at all", t, func() {
		planner := plannerForTest()

		out := planner.Step(tickerEnvelope("TEST/USD", 100, 101))
		decision := out.StrategyRound.Decisions[0]

		Convey("there is nothing to model the transition with", func() {
			So(decision.Action, ShouldEqual, types.ActionNothing)
			So(decision.PredictiveStatus, ShouldEqual, "awaiting-advisor-consensus")
		})
	})
}

func TestPlannerProjectsTheDeliberation(t *testing.T) {
	Convey("Given a deliberating War Room", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
			advisorPerspective("auction", "SellersAbsorbing", 0.9),
		}

		out := planner.Step(envelope)
		decision := out.StrategyRound.Decisions[0]

		Convey("the consensus distribution reaches the decision surface", func() {
			So(decision.Alternatives["consensus:participants"], ShouldEqual, 2)
			So(decision.Alternatives["consensus:vetoes"], ShouldBeGreaterThan, 0)

			Convey("including the per-move probabilities", func() {
				So(decision.Alternatives, ShouldContainKey, "move:explosive_pump")
				So(decision.Alternatives, ShouldContainKey, "move:flash_dump")
			})
		})
	})
}

func TestPlannerCarriesOpportunityContext(t *testing.T) {
	Convey("Given an armed opportunity on the ticker's symbol", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Opportunities = []*types.OpportunityCandidate{
			{
				Symbol:    "TEST/USD",
				Archetype: types.ArchetypeVerticalIgnition,
				Phase:     types.PhaseArmed,
			},
		}

		out := planner.Step(envelope)
		decision := out.StrategyRound.Decisions[0]

		Convey("the decision reports that opportunity's archetype and phase", func() {
			So(decision.Opportunity, ShouldBeTrue)
			So(decision.OpportunityType, ShouldEqual, string(types.ArchetypeVerticalIgnition))
			So(decision.OpportunityPhase, ShouldEqual, string(types.PhaseArmed))
		})
	})

	Convey("Given an opportunity for a different symbol", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Opportunities = []*types.OpportunityCandidate{
			{Symbol: "OTHER/USD", Archetype: types.ArchetypeVerticalIgnition},
		}

		out := planner.Step(envelope)

		Convey("it is not attributed to this symbol's decision", func() {
			So(out.StrategyRound.Decisions[0].Opportunity, ShouldBeFalse)
		})
	})
}

func TestPlannerIgnoresNonTickerEnvelopes(t *testing.T) {
	Convey("Given a trade envelope", t, func() {
		planner := plannerForTest()

		out := planner.Step(&types.Envelope{TypeID: types.EnvelopeTrade})

		Convey("it passes through untouched with no decision round", func() {
			So(out, ShouldNotBeNil)
			So(out.StrategyRound, ShouldBeNil)
			So(planner.Error(), ShouldBeNil)
		})
	})
}
