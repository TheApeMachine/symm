package logic_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	logic "github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
marketOutcome retains analyzer effects across every cut in one semantic market
transition so transient classifications and settled state are both examined.
*/
type marketOutcome struct {
	symbols     map[string]bool
	edges       map[types.EdgeType]int
	winners     map[types.CategoryType]int
	minimumFlow float64
	maximumFlow float64
	advanced    int
	replayed    int
	cognitions  int
	forecasts   int
	forecastSum float64
	forecastBy  map[string]float64
	forecastN   map[string]int
	categories  int
}

/*
observeManifold verifies that the analyzer projects every simulated symbol into
one physically valid state and records whether the Hawkes epoch really advanced.
*/
func (outcome *marketOutcome) observeManifold(
	thesis *types.Thesis,
	replay bool,
) {
	count := 0

	thesis.Manifold.Range(func(_, value any) bool {
		state, valid := value.(manifold.State)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[state.Symbol], ShouldBeTrue)
		So(state.Source, ShouldEqual, "manifold")
		So(state.GasReady(), ShouldBeTrue)
		So(state.Replay, ShouldEqual, replay)
		So(state.BuyIntensity, ShouldBeGreaterThanOrEqualTo, 0)
		So(state.SellIntensity, ShouldBeGreaterThanOrEqualTo, 0)

		flow := state.BuyIntensity - state.SellIntensity
		outcome.minimumFlow = min(outcome.minimumFlow, flow)
		outcome.maximumFlow = max(outcome.maximumFlow, flow)
		count++

		if state.Replay {
			outcome.replayed++
			return true
		}

		outcome.advanced++
		return true
	})

	So(count, ShouldEqual, len(outcome.symbols))
}

/*
observeGraphs proves each symbol graph contains real, internally referenced
relationships and accounts for every relationship type composed by Analyzer.
*/
func (outcome *marketOutcome) observeGraphs(thesis *types.Thesis) {
	count := 0

	thesis.Graphs.Range(func(_, value any) bool {
		graph, valid := value.(*types.Graph)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[graph.Symbol], ShouldBeTrue)
		So(graph.Nodes(), ShouldNotBeEmpty)
		So(graph.Edges(), ShouldNotBeEmpty)
		nodes := make(map[string]bool, len(graph.Nodes()))

		for _, node := range graph.Nodes() {
			nodes[node.Key] = true
		}

		for _, edge := range graph.Edges() {
			So(nodes[edge.From], ShouldBeTrue)
			So(nodes[edge.To], ShouldBeTrue)
			So(edge.At.IsZero(), ShouldBeFalse)
			outcome.edges[edge.Type]++
		}

		count++
		return true
	})

	So(count, ShouldEqual, len(outcome.symbols))
}

/*
observeModels validates the analyzer's chronological resonance, causal, and
forecast outputs. Replayed Hawkes epochs must not manufacture new observations.
*/
func (outcome *marketOutcome) observeModels(
	thesis *types.Thesis,
	replay bool,
) {
	if replay {
		So(thesis.Resonance, ShouldBeEmpty)
		So(thesis.Causal, ShouldBeEmpty)
		So(thesis.Hypotheses, ShouldBeEmpty)
		So(thesis.Forecasts, ShouldBeEmpty)
		return
	}

	So(thesis.Resonance, ShouldHaveLength, len(outcome.symbols))
	So(thesis.Causal, ShouldHaveLength, len(outcome.symbols))
	So(thesis.Hypotheses, ShouldHaveLength, len(outcome.symbols))

	for _, value := range thesis.Resonance {
		reading, valid := value.(*logic.ResonanceOutcome)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[reading.Symbol], ShouldBeTrue)
		So(reading.Source, ShouldEqual, "resonance")
		So(reading.Samples, ShouldBeGreaterThan, 0)
		So(reading.IsFinite(), ShouldBeTrue)
	}

	for _, value := range thesis.Causal {
		reading, valid := value.(*logic.CausalOutcome)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[reading.Symbol], ShouldBeTrue)
		So(reading.Source, ShouldEqual, "causal")
		So(reading.Samples, ShouldBeGreaterThan, 0)
		So(reading.Treatment, ShouldNotBeEmpty)
		So(reading.Target, ShouldNotBeEmpty)
		So(reading.InformedFlow, ShouldBeBetweenOrEqual, 0, 1)
	}

	for _, hypothesis := range thesis.Hypotheses {
		So(hypothesis.Source, ShouldEqual, types.SourceCausal)
		So(outcome.symbols[hypothesis.Symbol], ShouldBeTrue)
		So(hypothesis.Samples, ShouldBeGreaterThan, 0)
		So(hypothesis.Claim, ShouldNotBeEmpty)
		So(hypothesis.Treatment, ShouldNotBeEmpty)
		So(hypothesis.Outcome, ShouldNotBeEmpty)
	}

	for _, forecast := range thesis.Forecasts {
		So(forecast.Eligible(), ShouldBeTrue)
		So(forecast.Source, ShouldEqual, "resonance+causal")
		So(outcome.symbols[forecast.Symbol], ShouldBeTrue)
		So(forecast.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
		So(math.IsNaN(forecast.ExpectedReturn), ShouldBeFalse)
		So(math.IsInf(forecast.ExpectedReturn, 0), ShouldBeFalse)
		outcome.forecastSum += forecast.ExpectedReturn
		outcome.forecastBy[forecast.Symbol] += forecast.ExpectedReturn
		outcome.forecastN[forecast.Symbol]++
		outcome.forecasts++
	}
}

/*
observeCognition checks every DMT reading belongs to the projected market and
ensures every category is the ready classification it claims to represent.
*/
func (outcome *marketOutcome) observeCognition(thesis *types.Thesis) {
	readings := make(map[string]types.Cognition)
	ready := 0

	thesis.Cognition.Range(func(_, value any) bool {
		reading, valid := value.(types.Cognition)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[reading.Symbol], ShouldBeTrue)
		So(reading.Source, ShouldEqual, "dmt")
		So(reading.At.IsZero(), ShouldBeFalse)
		So(reading.Sequence, ShouldNotBeEmpty)
		So(reading.Predictions, ShouldNotBeNil)
		So(reading.Branches, ShouldNotBeEmpty)
		So(reading.Beams, ShouldNotBeEmpty)
		So(reading.NodeCount, ShouldEqual, len(reading.Branches))
		So(reading.LookaheadPaths, ShouldEqual, len(reading.Beams))
		readings[reading.Symbol] = reading
		outcome.cognitions++

		if reading.Ready {
			ready++
			outcome.winners[types.CategoryType(reading.Winner)]++
		}

		return true
	})

	So(thesis.Categories, ShouldHaveLength, ready)

	for _, category := range thesis.Categories {
		reading, found := readings[category.Symbol]
		So(found, ShouldBeTrue)
		So(reading.Ready, ShouldBeTrue)
		So(category.Type, ShouldEqual, types.CategoryType(reading.Winner))
		So(category.Confidence, ShouldEqual, reading.Confidence)
		So(category.Surprisal, ShouldEqual, reading.EntropyBits)
		So(category.Strength, ShouldEqual, reading.LookaheadScore)
		So(category.Maturity, ShouldEqual, float64(reading.Cohort))
		outcome.categories++
	}
}

/*
TestAnalyzerUpdate drives the complete production graph through every current
directional, liquidity, cadence, displacement, cross-symbol, and replay regime
and verifies every Analyzer-owned Thesis surface against its causal state.
*/
func TestAnalyzerUpdate(t *testing.T) {
	proofs := []struct {
		name                              string
		state                             tests.MarketState
		steps                             int
		replay                            bool
		prepare                           bool
		minimumSign                       int
		maximumSign                       int
		forecastSign                      int
		buy                               int
		sell                              int
		balanced                          int
		supports, contradicts, conditions int
	}{
		{"baseline", tests.MarketStateBaseline, 16, false, false, -1, 1, 0, 32, 16, 0, 715, 468, 48},
		{"fast pump", tests.MarketStateFastPump, 12, false, false, 0, 1, 1, 31, 0, 5, 663, 410, 36},
		{"slow pump", tests.MarketStateSlowPump, 20, false, false, 0, 1, 1, 55, 0, 5, 1027, 644, 60},
		{"fast dump", tests.MarketStateFastDump, 12, false, false, -1, 0, -1, 0, 26, 10, 594, 378, 36},
		{"slow dump", tests.MarketStateSlowDump, 20, false, false, -1, 0, -1, 1, 49, 10, 892, 581, 60},
		{"volume absorption", tests.MarketStateVolumeAbsorption, 12, false, false, 0, 1, 0, 29, 0, 7, 336, 214, 21},
		{"low-volume lift", tests.MarketStateLowVolumeLift, 12, false, false, 0, 1, 1, 31, 0, 5, 633, 394, 36},
		{"spread compression", tests.MarketStateSpreadCompression, 12, false, false, -1, 1, 0, 24, 12, 0, 363, 225, 21},
		{"thin liquidity", tests.MarketStateThinLiquidity, 1, true, false, -1, 1, 0, 2, 1, 0, 47, 28, 0},
		{"loaded liquidity", tests.MarketStateLoadedLiquidity, 18, true, false, -1, 1, 0, 36, 18, 0, 522, 288, 0},
		{"liquidity retreat", tests.MarketStateLiquidityRetreat, 1, true, false, -1, 1, 0, 2, 1, 0, 59, 43, 0},
		{"spoof liquidity", tests.MarketStateSpoofLiquidity, 1, true, false, -1, 1, 0, 2, 1, 0, 32, 16, 0},
		{"depth thinning", tests.MarketStateDepthThinning, 1, true, false, -1, 1, 0, 2, 1, 0, 6, 3, 0},
		{"slow-cadence lift", tests.MarketStateSlowCadenceLift, 12, false, false, 0, 1, 1, 31, 0, 5, 663, 410, 36},
		{"small-displacement lift", tests.MarketStateSmallLift, 12, false, false, 0, 1, 1, 31, 0, 5, 661, 409, 36},
		{"spread control", tests.MarketStateSpreadControl, 12, false, false, 0, 1, 0, 31, 0, 5, 457, 333, 21},
		{"leader follower", tests.MarketStateLeaderFollower, 20, false, false, -1, 1, 1, 54, 6, 0, 1027, 646, 60},
		{"persistent adverse divergence", tests.MarketStateAdverseDivergence, 12, false, true, -1, 1, 1, 24, 12, 0, 616, 384, 36},
	}

	Convey("Given materially distinct fixture-driven markets", t, func() {
		outcomes := make(map[string]*marketOutcome, len(proofs))
		var marketSymbols []string

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			marketSymbols = market.Symbols
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)

			if proof.prepare {
				So(market.Transition(proof.state, tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			}

			outcome := &marketOutcome{
				symbols:     map[string]bool{},
				edges:       map[types.EdgeType]int{},
				winners:     map[types.CategoryType]int{},
				minimumFlow: math.Inf(1),
				maximumFlow: math.Inf(-1),
				forecastBy:  map[string]float64{},
				forecastN:   map[string]int{},
			}

			for _, symbol := range market.Symbols {
				outcome.symbols[symbol] = true
			}

			So(market.Transition(proof.state, func() error {
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}
				So(thesis.Incomplete(), ShouldBeFalse)
				So(thesis.Measurements, ShouldNotBeEmpty)
				So(thesis.At, ShouldResemble, market.Now())
				outcome.observeManifold(thesis, proof.replay)
				outcome.observeGraphs(thesis)
				outcome.observeModels(thesis, proof.replay)
				outcome.observeCognition(thesis)
				return nil
			}), ShouldBeNil)

			outcomes[proof.name] = outcome
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should preserve direction, chronology, and evidence semantics", func() {
			for _, proof := range proofs {
				outcome := outcomes[proof.name]
				expectedObservations := proof.steps * len(marketSymbols)
				expectedAdvanced := expectedObservations
				expectedReplayed := 0

				if proof.replay {
					expectedAdvanced = 0
					expectedReplayed = expectedObservations
				}

				So(outcome.minimumFlow == 0, ShouldEqual, proof.minimumSign == 0)
				So(math.Signbit(outcome.minimumFlow), ShouldEqual, proof.minimumSign < 0)
				So(outcome.maximumFlow == 0, ShouldEqual, proof.maximumSign == 0)
				So(math.Signbit(outcome.maximumFlow), ShouldEqual, proof.maximumSign < 0)
				So(outcome.cognitions, ShouldEqual, expectedObservations)
				So(outcome.categories, ShouldEqual, expectedObservations)
				So(outcome.winners["buy"], ShouldEqual, proof.buy)
				So(outcome.winners["sell"], ShouldEqual, proof.sell)
				So(outcome.winners["balanced"], ShouldEqual, proof.balanced)
				So(outcome.advanced, ShouldEqual, expectedAdvanced)
				So(outcome.replayed, ShouldEqual, expectedReplayed)

				So(outcome.edges[types.Supports], ShouldEqual, proof.supports)
				So(outcome.edges[types.Contradicts], ShouldEqual, proof.contradicts)
				So(outcome.edges[types.Conditions], ShouldEqual, proof.conditions)

				if proof.forecastSign == 0 {
					So(outcome.forecasts, ShouldEqual, 0)
					So(outcome.forecastSum, ShouldEqual, 0.0)
				}

				if proof.forecastSign != 0 {
					So(outcome.forecasts == 0, ShouldBeFalse)
					So(outcome.forecastSum == 0, ShouldBeFalse)
					So(math.Signbit(outcome.forecastSum), ShouldEqual, proof.forecastSign < 0)
				}

				for _, symbol := range marketSymbols {
					if proof.forecastSign == 0 {
						So(outcome.forecastN[symbol], ShouldEqual, 0)
						So(outcome.forecastBy[symbol], ShouldEqual, 0.0)
					}
				}

				if proof.state != tests.MarketStateAdverseDivergence {
					continue
				}

				for index, symbol := range marketSymbols {
					expectedSign := 1

					if index == 0 {
						expectedSign = -1
					}

					So(outcome.forecastN[symbol] == 0, ShouldBeFalse)
					So(outcome.forecastBy[symbol] == 0, ShouldBeFalse)
					So(math.Signbit(outcome.forecastBy[symbol]), ShouldEqual, expectedSign < 0)
				}
			}
		})
	})
}

/*
BenchmarkAnalyzerUpdate measures Analyzer through the same fixture-to-Crypto
production path used by the market proof.
*/
func BenchmarkAnalyzerUpdate(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if err := wired.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	defer market.Close()

	if err := market.Warmup(tests.Consume(wired.Crypto.Tick)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := market.Transition(tests.MarketStateBaseline, tests.Consume(wired.Crypto.Tick)); err != nil {
			b.Fatal(err)
		}
	}
}
