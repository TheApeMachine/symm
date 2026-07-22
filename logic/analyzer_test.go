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
func (outcome *marketOutcome) observeCognition(
	thesis *types.Thesis,
	focus string,
) {
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
		readings[reading.Symbol] = reading
		outcome.cognitions++

		if reading.Symbol == focus {
			So(reading.Branches, ShouldNotBeEmpty)
			So(reading.Beams, ShouldNotBeEmpty)
			So(reading.NodeCount, ShouldEqual, len(reading.Branches))
			So(reading.LookaheadPaths, ShouldEqual, len(reading.Beams))
		}

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
TestAnalyzerUpdate drives the complete production graph through quiet,
directional, and replay-only markets and verifies every Analyzer-owned Thesis
surface against the causal state that produced it.
*/
func TestAnalyzerUpdate(t *testing.T) {
	proofs := []struct {
		name    string
		state   tests.MarketState
		replay  bool
		prepare bool
	}{
		{"baseline", tests.MarketStateBaseline, false, false},
		{"fast pump", tests.MarketStateFastPump, false, false},
		{"fast dump", tests.MarketStateFastDump, false, false},
		{"persistent adverse divergence", tests.MarketStateAdverseDivergence, false, true},
		{"book-only replay", tests.MarketStateThinLiquidity, true, false},
	}

	Convey("Given materially distinct fixture-driven markets", t, func() {
		outcomes := make(map[string]*marketOutcome, len(proofs))
		var marketSymbols []string

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)

			if marketSymbols == nil {
				marketSymbols = append([]string(nil), market.Symbols...)
			}

			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			focus := market.Symbols[0]
			wired.Analyzer.Focus(focus)
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
				outcome.observeCognition(thesis, focus)
				return nil
			}), ShouldBeNil)

			outcomes[proof.name] = outcome
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should preserve direction, chronology, and evidence semantics", func() {
			for _, name := range []string{
				"baseline", "fast pump", "fast dump", "persistent adverse divergence",
			} {
				outcome := outcomes[name]
				So(outcome.advanced, ShouldBeGreaterThan, 0)
				So(outcome.replayed, ShouldEqual, 0)
				So(outcome.edges[types.Supports], ShouldBeGreaterThan, 0)
				So(outcome.edges[types.Contradicts], ShouldBeGreaterThan, 0)
				So(outcome.edges[types.Conditions], ShouldBeGreaterThan, 0)
				So(outcome.cognitions, ShouldBeGreaterThan, 0)
			}

			baseline := outcomes["baseline"]
			So(baseline.minimumFlow, ShouldBeLessThan, 0)
			So(baseline.maximumFlow, ShouldBeGreaterThan, 0)
			So(baseline.forecasts, ShouldEqual, 0)

			pump := outcomes["fast pump"]
			So(pump.minimumFlow, ShouldBeGreaterThanOrEqualTo, 0)
			So(pump.maximumFlow, ShouldBeGreaterThan, 0)
			So(pump.forecasts, ShouldBeGreaterThan, 0)
			So(pump.forecastSum/float64(pump.forecasts), ShouldBeGreaterThan, 0)
			So(pump.winners["sell"], ShouldEqual, 0)

			dump := outcomes["fast dump"]
			So(dump.minimumFlow, ShouldBeLessThan, 0)
			So(dump.maximumFlow, ShouldBeLessThanOrEqualTo, 0)
			So(dump.forecasts, ShouldBeGreaterThan, 0)
			So(dump.forecastSum/float64(dump.forecasts), ShouldBeLessThan, 0)
			So(
				pump.forecastSum/float64(pump.forecasts),
				ShouldBeGreaterThan,
				dump.forecastSum/float64(dump.forecasts),
			)
			So(dump.winners["buy"], ShouldEqual, 0)

			divergence := outcomes["persistent adverse divergence"]
			So(divergence.minimumFlow, ShouldBeLessThan, 0)
			So(divergence.maximumFlow, ShouldBeGreaterThan, 0)
			So(divergence.forecasts, ShouldBeGreaterThan, 0)
			So(divergence.forecastN[marketSymbols[0]], ShouldBeGreaterThan, 0)
			So(
				divergence.forecastBy[marketSymbols[0]]/
					float64(divergence.forecastN[marketSymbols[0]]),
				ShouldBeLessThan,
				0,
			)

			for _, symbol := range marketSymbols[1:] {
				So(divergence.forecastN[symbol], ShouldBeGreaterThan, 0)
				So(
					divergence.forecastBy[symbol]/
						float64(divergence.forecastN[symbol]),
					ShouldBeGreaterThan,
					0,
				)
			}

			replay := outcomes["book-only replay"]
			So(replay.advanced, ShouldEqual, 0)
			So(replay.replayed, ShouldEqual, len(replay.symbols))
			So(replay.edges[types.Supports], ShouldBeGreaterThan, 0)
			So(replay.edges[types.Contradicts], ShouldBeGreaterThan, 0)
			So(replay.edges[types.Conditions], ShouldEqual, 0)
			So(replay.forecasts, ShouldEqual, 0)
			// Focus-only recall may republish a Ready winner from the trained
			// tree; replay must not invent forecasts or multi-symbol cognition.
			So(replay.categories, ShouldBeLessThanOrEqualTo, 1)
			So(replay.cognitions, ShouldEqual, 1)
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
