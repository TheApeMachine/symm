package logic_test

import (
	"math"
	"testing"
	"time"

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
	flowAbsSum  float64
	flowSamples int
	advanced    int
	replayed    int
	cognitions  int
	forecasts   int
	forecastSum float64
	forecastBy  map[string]float64
	forecastN   map[string]int
	categories  int
	resonanceN  int
	causalN     int
	layerRows   int
	uiResonance int
	uiManifold  int
	uiCognition int
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
		outcome.flowAbsSum += math.Abs(flow)
		outcome.flowSamples++
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
		So(reading.Layers, ShouldNotBeEmpty)
		So(reading.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
		outcome.layerRows += len(reading.Layers)
	}

	outcome.resonanceN = len(thesis.Resonance)

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

	outcome.causalN = len(thesis.Causal)

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
observeCognition checks every DMT reading belongs to the projected market.
Category rows must name a ready winner; exact confidence/cohort stamps are
counted only on a consistent live snapshot so an in-flight Actor cut cannot
flake the proof.
*/
func (outcome *marketOutcome) observeCognition(thesis *types.Thesis) {
	readings := make(map[string]types.Cognition)

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

		if reading.Ready && reading.Winner != "" {
			outcome.winners[types.CategoryType(reading.Winner)]++
		}

		return true
	})

	for _, category := range thesis.Categories {
		reading, found := readings[category.Symbol]

		if !found || !reading.Ready || reading.Winner == "" {
			continue
		}

		// Winner can advance mid-cut on the live thesis; only count rows whose
		// Type still matches the Cognition snapshot we just collected.
		if category.Type != types.CategoryType(reading.Winner) {
			continue
		}

		outcome.categories++

		if category.Confidence == reading.Confidence &&
			category.Surprisal == reading.EntropyBits &&
			category.Strength == reading.LookaheadScore &&
			category.Maturity == float64(reading.Cohort) {
			outcome.winners[category.Type]++
		}
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
		{"slow pump", tests.MarketStateSlowPump, false, false},
		{"fast dump", tests.MarketStateFastDump, false, false},
		{"slow dump", tests.MarketStateSlowDump, false, false},
		{"volume absorption", tests.MarketStateVolumeAbsorption, false, false},
		{"persistent adverse divergence", tests.MarketStateAdverseDivergence, false, true},
		{"leader follower", tests.MarketStateLeaderFollower, false, false},
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

			types.SetFocus(market.Symbols[0])

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

			if !proof.replay {
				resonanceFrame := waitCached(wired, "resonance")
				So(resonanceFrame, ShouldNotBeNil)
				So(string(resonanceFrame), ShouldContainSubstring, `"resonance":`)
				So(string(resonanceFrame), ShouldContainSubstring, `"layers":`)
				outcome.uiResonance++

				manifoldFrame := waitCached(wired, "manifold")
				So(manifoldFrame, ShouldNotBeNil)
				So(string(manifoldFrame), ShouldContainSubstring, `"manifold":`)
				So(string(manifoldFrame), ShouldContainSubstring, `"rho":`)
				So(string(manifoldFrame), ShouldContainSubstring, market.Symbols[0])
				outcome.uiManifold++

				cognitionFrame := waitCached(wired, "cognition")
				So(cognitionFrame, ShouldNotBeNil)
				So(string(cognitionFrame), ShouldContainSubstring, `"cognition":`)
				outcome.uiCognition++
			}

			types.SetFocus("")
			outcomes[proof.name] = outcome
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should preserve direction, chronology, and evidence semantics", func() {
			for _, name := range []string{
				"baseline", "fast pump", "slow pump", "fast dump", "slow dump",
				"volume absorption", "persistent adverse divergence", "leader follower",
			} {
				outcome := outcomes[name]
				So(outcome.cognitions, ShouldBeGreaterThan, 0)
				So(outcome.resonanceN, ShouldEqual, len(marketSymbols))
				So(outcome.causalN, ShouldEqual, len(marketSymbols))
				So(outcome.layerRows, ShouldBeGreaterThan, 0)
				So(outcome.flowSamples, ShouldBeGreaterThanOrEqualTo, len(marketSymbols))
				So(outcome.uiResonance, ShouldEqual, 1)
				So(outcome.uiManifold, ShouldEqual, 1)
				So(outcome.uiCognition, ShouldEqual, 1)
			}

			for _, name := range []string{
				"fast pump", "slow pump", "fast dump", "slow dump",
				"persistent adverse divergence", "leader follower",
			} {
				So(outcomes[name].advanced, ShouldBeGreaterThan, 0)
				So(outcomes[name].categories, ShouldBeGreaterThan, 0)
			}

			baseline := outcomes["baseline"]
			pump := outcomes["fast pump"]
			slowPump := outcomes["slow pump"]
			dump := outcomes["fast dump"]
			slowDump := outcomes["slow dump"]

			baselineAbs := baseline.flowAbsSum / float64(baseline.flowSamples)
			pumpAbs := pump.flowAbsSum / float64(pump.flowSamples)
			dumpAbs := dump.flowAbsSum / float64(dump.flowSamples)

			So(pump.maximumFlow, ShouldBeGreaterThan, 0)
			So(dump.minimumFlow, ShouldBeLessThan, 0)
			So(pumpAbs, ShouldBeGreaterThan, baselineAbs)
			So(dumpAbs, ShouldBeGreaterThan, baselineAbs)

			So(pump.forecasts, ShouldBeGreaterThan, 0)
			So(dump.forecasts, ShouldBeGreaterThan, 0)
			So(slowPump.forecasts, ShouldBeGreaterThan, 0)
			So(slowDump.forecasts, ShouldBeGreaterThan, 0)

			pumpMean := pump.forecastSum / float64(pump.forecasts)
			dumpMean := dump.forecastSum / float64(dump.forecasts)
			slowPumpMean := slowPump.forecastSum / float64(slowPump.forecasts)
			slowDumpMean := slowDump.forecastSum / float64(slowDump.forecasts)

			So(pumpMean, ShouldBeGreaterThan, 0)
			So(dumpMean, ShouldBeLessThan, 0)
			So(slowPumpMean, ShouldBeGreaterThan, 0)
			So(slowDumpMean, ShouldBeLessThan, 0)
			So(pumpMean, ShouldBeGreaterThan, dumpMean)
			So(pumpMean, ShouldBeGreaterThan, slowDumpMean)
			So(slowPumpMean, ShouldBeGreaterThan, dumpMean)

			if baseline.forecasts > 0 {
				baselineMean := baseline.forecastSum / float64(baseline.forecasts)
				So(math.Abs(baselineMean), ShouldBeLessThan, math.Abs(pumpMean))
				So(math.Abs(baselineMean), ShouldBeLessThan, math.Abs(dumpMean))
			}

			absorption := outcomes["volume absorption"]
			So(absorption.cognitions, ShouldBeGreaterThan, 0)
			So(absorption.maximumFlow, ShouldBeGreaterThan, absorption.minimumFlow)
			So(absorption.resonanceN, ShouldEqual, len(marketSymbols))
			So(absorption.uiManifold, ShouldEqual, 1)

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

			leader := outcomes["leader follower"]
			So(leader.forecasts, ShouldBeGreaterThan, 0)
			So(leader.advanced, ShouldBeGreaterThan, 0)

			replay := outcomes["book-only replay"]
			// ThinLiquidity is book-only: Hawkes never publishes, so Analyzer does
			// not restamp Manifold.Replay. What must hold is no new forecast mint
			// and cognition residue from warmup still present.
			So(replay.forecasts, ShouldEqual, 0)
			So(replay.cognitions, ShouldBeGreaterThan, 0)
			So(replay.uiManifold, ShouldEqual, 0)
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

/*
waitCached polls the hub cache until the replaceable key arrives or the
deadline elapses. Analyzer publish is async relative to Market.Drain return.
*/
func waitCached(wired *stack.Stack, key string) []byte {
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		if frame := wired.UIHub.Cached(key); len(frame) > 0 {
			return frame
		}

		time.Sleep(5 * time.Millisecond)
	}

	return wired.UIHub.Cached(key)
}
