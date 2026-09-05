package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/mcts"
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
	return &Planner{
		warRoom:        advisor.NewWarRoom(),
		lastEpochs:     make(map[string]hindsight.StreamEpoch),
		lastSequences:  make(map[string]uint64),
		lastTimestamps: make(map[string]time.Time),
	}
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

func TestPlannerRequiresResonance(t *testing.T) {
	Convey("Given advisors but no calibrated resonance forecast", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
		}

		out := planner.Step(envelope)
		decision := out.StrategyRound.Decisions[0]

		Convey("planning halts because resonance is strictly required", func() {
			So(decision.PredictiveStatus, ShouldEqual, "resonance-missing")
			So(decision.Action, ShouldEqual, types.ActionNothing)
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

func TestPlannerRetainsTradePerspectivesAcrossTickerStep(t *testing.T) {
	Convey("Given a trade envelope carrying fresh advisor perspectives", t, func() {
		planner := plannerForTest()
		tradeEnv := &types.Envelope{
			TypeID: types.EnvelopeTrade,
			TradeData: kraken.TradeData{
				Symbol: "TEST/USD",
			},
			Perspectives: []*types.Perspective{
				advisorPerspective("momentum", "Building", 0.8),
				advisorPerspective("auction", "BuyersBreakingThrough", 0.85),
			},
		}

		outTrade := planner.Step(tradeEnv)
		So(outTrade, ShouldNotBeNil)
		So(outTrade.StrategyRound, ShouldBeNil)

		Convey("when a later ticker envelope arrives with empty perspectives", func() {
			tickEnv := tickerEnvelope("TEST/USD", 100, 101)
			So(tickEnv.Perspectives, ShouldBeNil)

			outTick := planner.Step(tickEnv)
			So(outTick, ShouldNotBeNil)
			So(outTick.StrategyRound, ShouldNotBeNil)

			decision := outTick.StrategyRound.Decisions[0]
			Convey("the resident council deliberates with the retained perspectives", func() {
				So(decision.PredictiveStatus, ShouldNotEqual, "awaiting-advisor-consensus")
				So(decision.Alternatives["consensus:participants"], ShouldEqual, 2)
			})
		})
	})
}

func TestPlannerForecastHorizonDecoupled(t *testing.T) {
	Convey("Given a planner and a ticker without calibrated resonance", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)

		out := planner.Step(envelope)

		Convey("the decision forecast horizon defaults to 0 rather than searchHorizon", func() {
			So(out, ShouldNotBeNil)
			So(out.StrategyRound, ShouldNotBeNil)
			So(out.StrategyRound.Decisions[0].ForecastHorizon, ShouldEqual, 0)
		})
	})

	Convey("Given a planner and a ticker with calibrated resonance", t, func() {
		planner := plannerForTest()
		envelope := tickerEnvelope("TEST/USD", 100, 101)
		envelope.Resonance = &types.ResonanceArtifact{
			Calibrated:       true,
			SupportedHorizon: 45,
			Confidence:       0.9,
			Forecast: &types.ResonanceReturnForecast{
				Call:    1,
				Horizon: 45,
				Distribution: learning.RLSOutput{
					Ready: true,
					Scale: 0.05,
				},
			},
		}

		out := planner.Step(envelope)

		Convey("the decision forecast horizon reflects the resonance supported horizon", func() {
			So(out, ShouldNotBeNil)
			So(out.StrategyRound, ShouldNotBeNil)
			So(out.StrategyRound.Decisions[0].ForecastHorizon, ShouldEqual, 45)
		})
	})
}

func TestPlannerEntryCosts(t *testing.T) {
	Convey("Given a planner with no desk price provider", t, func() {
		planner := plannerForTest()
		costs := planner.entryCosts("TEST/USD", 10, mcts.CostModel{FeeRate: 0.002})

		Convey("it returns base costs untouched", func() {
			So(costs.FeeRate, ShouldEqual, 0.002)
			So(costs.SlippageFraction, ShouldEqual, 0)
		})
	})
}

func TestPlannerStreamingTopologyCases(t *testing.T) {
	Convey("Case K: Deterministic replay producing identical decision and trace", t, func() {
		planner1 := plannerForTest()
		planner2 := plannerForTest()

		env1 := tickerEnvelope("TEST/USD", 100, 101)
		env1.Tick = 4242
		env1.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
		}

		env2 := tickerEnvelope("TEST/USD", 100, 101)
		env2.Tick = 4242
		env2.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
		}

		out1 := planner1.Step(env1)
		out2 := planner2.Step(env2)

		So(out1.StrategyRound.Decisions[0].PredictiveStatus, ShouldEqual, out2.StrategyRound.Decisions[0].PredictiveStatus)
		So(out1.StrategyRound.Decisions[0].Reason, ShouldEqual, out2.StrategyRound.Decisions[0].Reason)
	})

	Convey("Case L: Missing required input halts without fallback", t, func() {
		planner := plannerForTest()
		env := tickerEnvelope("TEST/USD", 100, 101)
		env.Perspectives = []*types.Perspective{
			advisorPerspective("momentum", "Building", 0.8),
		}

		out := planner.Step(env)
		So(out.StrategyRound.Decisions[0].PredictiveStatus, ShouldEqual, "resonance-missing")
		So(out.StrategyRound.Decisions[0].Action, ShouldEqual, types.ActionNothing)
	})
}

func TestPlannerEpochAndSequenceValidation(t *testing.T) {
	Convey("Given a planner receiving stream envelopes", t, func() {
		planner := plannerForTest()

		Convey("sequence progression within the same epoch succeeds", func() {
			first := tickerEnvelope("TEST/USD", 100, 101)
			first.Stream = hindsight.StreamRef{Epoch: 1, Sequence: 100}
			first.TickerData.Timestamp = time.Unix(10, 0)
			So(planner.Step(first), ShouldNotBeNil)

			second := tickerEnvelope("TEST/USD", 100, 101)
			second.Stream = hindsight.StreamRef{Epoch: 1, Sequence: 101}
			second.TickerData.Timestamp = time.Unix(11, 0)
			So(planner.Step(second), ShouldNotBeNil)

			Convey("sequence regression within the same epoch halts the planner", func() {
				regressed := tickerEnvelope("TEST/USD", 100, 101)
				regressed.Stream = hindsight.StreamRef{Epoch: 1, Sequence: 50}
				regressed.TickerData.Timestamp = time.Unix(12, 0)
				So(planner.Step(regressed), ShouldBeNil)
				So(planner.Error(), ShouldNotBeNil)
				So(planner.Error().Error(), ShouldContainSubstring, "sequence regression")
			})
		})

		Convey("reconnect advancing StreamEpoch resets sequence baseline cleanly", func() {
			first := tickerEnvelope("TEST/USD", 100, 101)
			first.Stream = hindsight.StreamRef{Epoch: 1, Sequence: 17436}
			first.TickerData.Timestamp = time.Unix(100, 0)
			So(planner.Step(first), ShouldNotBeNil)

			reconnect := tickerEnvelope("TEST/USD", 100, 101)
			reconnect.Stream = hindsight.StreamRef{Epoch: 2, Sequence: 1}
			reconnect.TickerData.Timestamp = time.Unix(101, 0)
			So(planner.Step(reconnect), ShouldNotBeNil)
			So(planner.Error(), ShouldBeNil)

			second := tickerEnvelope("TEST/USD", 100, 101)
			second.Stream = hindsight.StreamRef{Epoch: 2, Sequence: 2}
			second.TickerData.Timestamp = time.Unix(102, 0)
			So(planner.Step(second), ShouldNotBeNil)
			So(planner.Error(), ShouldBeNil)
		})

		Convey("epoch regression halts the planner", func() {
			first := tickerEnvelope("TEST/USD", 100, 101)
			first.Stream = hindsight.StreamRef{Epoch: 2, Sequence: 10}
			first.TickerData.Timestamp = time.Unix(10, 0)
			So(planner.Step(first), ShouldNotBeNil)

			staleEpoch := tickerEnvelope("TEST/USD", 100, 101)
			staleEpoch.Stream = hindsight.StreamRef{Epoch: 1, Sequence: 100}
			staleEpoch.TickerData.Timestamp = time.Unix(11, 0)
			So(planner.Step(staleEpoch), ShouldBeNil)
			So(planner.Error(), ShouldNotBeNil)
			So(planner.Error().Error(), ShouldContainSubstring, "epoch regression")
		})
	})
}

