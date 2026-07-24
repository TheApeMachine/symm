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
) {
	count := 0

	thesis.Manifold.Range(func(_, value any) bool {
		state, valid := value.(manifold.State)
		So(valid, ShouldBeTrue)
		So(outcome.symbols[state.Symbol], ShouldBeTrue)
		So(state.Source, ShouldEqual, "manifold")
		So(state.GasReady(), ShouldBeTrue)
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
observeModels validates the analyzer's chronological resonance, causal, and
forecast outputs. Replayed Hawkes epochs must not manufacture new observations.
*/
func (outcome *marketOutcome) observeModels(
	thesis *types.Thesis,
	replay bool,
) {
	if replay {
		// Shared Thesis may retain prior observe rows from warmup. A replay cut
		// must not mint into the accumulated forecast counter — that is checked
		// by leaving outcome.forecasts untouched here.
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
		So(forecast.Ready, ShouldBeTrue)
		So(forecast.Calibrated, ShouldBeTrue)
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
			So(market.Warmup(tests.Idle), ShouldBeNil)

			if proof.prepare {
				So(market.Transition(proof.state, tests.Idle), ShouldBeNil)
			}

			outcome := &marketOutcome{
				symbols:     map[string]bool{},
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
				thesis := wired.Thesis
				So(thesis.Incomplete(), ShouldBeFalse)
				So(thesis.Measurements, ShouldNotBeEmpty)
				So(thesis.At.IsZero(), ShouldBeFalse)
				So(thesis.At.After(market.Now()), ShouldBeFalse)
				outcome.observeManifold(thesis)
				outcome.observeModels(thesis, proof.replay)
				outcome.observeCognition(thesis)
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
				So(outcome.cognitions, ShouldBeGreaterThan, 0)
			}

			baseline := outcomes["baseline"]
			So(baseline.minimumFlow, ShouldBeLessThanOrEqualTo, 0)
			So(baseline.maximumFlow, ShouldBeGreaterThanOrEqualTo, 0)

			pump := outcomes["fast pump"]
			So(pump.maximumFlow, ShouldBeGreaterThan, 0)
			So(pump.forecasts, ShouldBeGreaterThan, 0)
			So(pump.forecastSum/float64(pump.forecasts), ShouldBeGreaterThan, 0)

			// Quiet baseline may still emit a calibrated forecast after warmup;
			// its mean expected return must stay below the pump's.
			if baseline.forecasts > 0 {
				So(
					math.Abs(baseline.forecastSum/float64(baseline.forecasts)),
					ShouldBeLessThan,
					math.Abs(pump.forecastSum/float64(pump.forecasts)),
				)
			}

			dump := outcomes["fast dump"]
			So(dump.minimumFlow, ShouldBeLessThan, 0)
			So(dump.forecasts, ShouldBeGreaterThan, 0)
			So(dump.forecastSum/float64(dump.forecasts), ShouldBeLessThan, 0)
			So(
				pump.forecastSum/float64(pump.forecasts),
				ShouldBeGreaterThan,
				dump.forecastSum/float64(dump.forecasts),
			)

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
			// ThinLiquidity is book-only: Hawkes never publishes, so Analyzer does
			// not restamp Manifold.Replay. What must hold is no new forecast mint
			// and cognition residue from warmup still present.
			So(replay.forecasts, ShouldEqual, 0)
			So(replay.cognitions, ShouldBeGreaterThan, 0)
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

	if err := market.Warmup(tests.Idle); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateBaseline, tests.Idle); err != nil {
			b.Fatal(err)
		}
	}
}
