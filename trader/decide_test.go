package trader

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

/*
manifoldMeasurement builds a manifold measurement artifact carrying the field
readout the solver would publish for one symbol.
*/
func manifoldMeasurement(
	symbol string,
	coherence, guidance, viscosity, pressure float64,
) *datura.Artifact {
	measurement := datura.Acquire("manifold", datura.APPJSON)
	measurement.WithScope(symbol)
	_ = measurement.SetOrigin(string(logic.SourceManifold))
	measurement.MergeOutput("coherenceMag2", coherence)
	measurement.MergeOutput("guidanceSpeed", guidance)
	measurement.MergeOutput("viscosityProxy", viscosity)
	measurement.MergeOutput("pressureGradNorm", pressure)

	return measurement
}

/*
causalMeasurement builds a causal measurement carrying the signed counterfactual
uplift the causal ladder published for one symbol.
*/
func causalMeasurement(symbol string, uplift float64) *datura.Artifact {
	measurement := datura.Acquire("causal", datura.APPJSON)
	measurement.WithScope(symbol)
	_ = measurement.SetOrigin(string(logic.SourceCausal))
	measurement.MergeOutput("uplift", uplift)
	measurement.MergeOutput("counterfactualReady", true)

	return measurement
}

func testDecider() *Decider {
	viper.Set("market.story.forward_return_min_samples", 30)
	viper.Set("trading.edge_min_bps", 10.0)

	return &Decider{economics: executionEconomics{
		takerFeeBps: 40,
		makerFeeBps: 25,
		slippageBps: 2,
	}}
}

/*
candidate builds a story action artifact the way market.Story.Update serializes
one: side as role, symbol as scope, the Action JSON as payload.
*/
func candidate(
	symbol string,
	side logic.Side,
	actionType logic.ActionType,
	confidence float64,
) *datura.Artifact {
	buf, _ := sonic.Marshal(logic.Action{
		Type:            actionType,
		Side:            side,
		Symbol:          symbol,
		EntryConfidence: confidence,
	})

	action := datura.Acquire("story", datura.APPJSON)
	action.WithRole(string(side))
	action.WithScope(symbol)
	action.WithPayload(buf)
	action.Poke(100.0, "decision", "expected_return_bps")
	action.Poke(30, "decision", "sample_count")
	action.Poke("test_forward_return", "decision", "edge_source")
	action.Poke(
		strings.ToLower(
			fmt.Sprintf("test|%s|%s|%s", symbol, side, actionType),
		),
		"decision",
		"setup_key",
	)

	return action
}

func TestDeciderRanksByFieldEdgeAndGatesRisk(t *testing.T) {
	decider := testDecider()

	// COHERENT: high coherence + guidance, low viscosity/pressure → strong edge.
	// WEAK: same kind of move but less coherent → smaller positive edge.
	// TRAP: viscous, incoherent, ruptured → field precision down-weights it.
	// GHOST: no field readout; missing deferred field evidence should not block
	// a playbook candidate that carries its own measured confidence.
	// The first three carry a positive causal counterfactual; only the measured
	// field precision differs, so field ranking is what the test
	// exercises.
	measurements := []*datura.Artifact{
		manifoldMeasurement("COHERENT/USD", 0.9, 0.8, 0.1, 0.05),
		manifoldMeasurement("WEAK/USD", 0.5, 0.5, 0.2, 0.05),
		manifoldMeasurement("TRAP/USD", 0.1, 0.2, 0.9, 0.7),
		causalMeasurement("COHERENT/USD", 1.0),
		causalMeasurement("WEAK/USD", 1.0),
		causalMeasurement("TRAP/USD", 1.0),
		causalMeasurement("GHOST/USD", 1.0),
	}

	exit := candidate("HELD/USD", logic.SideSell, logic.ActionSettlePosition, 0)

	actions := []*datura.Artifact{
		candidate("WEAK/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("COHERENT/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("TRAP/USD", logic.SideBuy, logic.ActionMarket, 0.9),
		candidate("GHOST/USD", logic.SideBuy, logic.ActionMarket, 0.9),
		exit,
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	// Protective exit always survives.
	if !containsSymbol(symbols, "HELD/USD") {
		t.Fatalf("protective exit was dropped: chosen=%v", symbols)
	}

	if !containsSymbol(symbols, "GHOST/USD") {
		t.Fatalf("entry with no deferred field readout was blocked: chosen=%v", symbols)
	}

	// Both viable entries survive, with the more coherent one ranked first.
	coherentRank := indexOf(symbols, "COHERENT/USD")
	weakRank := indexOf(symbols, "WEAK/USD")

	if coherentRank < 0 || weakRank < 0 {
		t.Fatalf("a viable entry was dropped: chosen=%v", symbols)
	}

	if coherentRank > weakRank {
		t.Fatalf(
			"more coherent entry ranked below weaker one: chosen=%v",
			symbols,
		)
	}

	trapRank := indexOf(symbols, "TRAP/USD")
	if trapRank < 0 {
		t.Fatalf("field-risk entry should be down-ranked, not silently dropped: chosen=%v", symbols)
	}

	if trapRank < coherentRank {
		t.Fatalf("field-risk entry ranked above coherent one: chosen=%v", symbols)
	}
}

/*
resonanceMeasurement builds a resonance measurement carrying reconstruction
surprise for one symbol, the way signal/resonance settles it.
*/
func resonanceMeasurement(symbol string, surprise float64) *datura.Artifact {
	measurement := datura.Acquire("resonance", datura.APPJSON)
	measurement.WithScope(symbol)
	_ = measurement.SetOrigin(string(logic.SourceResonance))
	measurement.MergeOutput("surprise", surprise)

	return measurement
}

func TestDeciderResonanceSurpriseLowersPrecision(t *testing.T) {
	decider := testDecider()

	// Identical field edge and entry confidence on both symbols; the only
	// difference is reconstruction surprise. The well-reconstructed symbol must
	// outrank the surprising one, because we trust the forward roll less where
	// the resonance model cannot explain the symbol.
	measurements := []*datura.Artifact{
		manifoldMeasurement("CALM/USD", 0.8, 0.7, 0.1, 0.05),
		manifoldMeasurement("WILD/USD", 0.8, 0.7, 0.1, 0.05),
		causalMeasurement("CALM/USD", 1.0),
		causalMeasurement("WILD/USD", 1.0),
		resonanceMeasurement("CALM/USD", 0.05),
		resonanceMeasurement("WILD/USD", 5.0),
	}

	actions := []*datura.Artifact{
		candidate("WILD/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("CALM/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	if indexOf(symbols, "CALM/USD") > indexOf(symbols, "WILD/USD") {
		t.Fatalf(
			"high-surprise symbol was not down-weighted: chosen=%v",
			symbols,
		)
	}
}

func TestDeciderCausalUpliftDrivesAndGates(t *testing.T) {
	decider := testDecider()

	// Identical field and confidence on all three; the causal counterfactual is
	// a ranking feature only. STRONG > WEAK by uplift, while FLAT is not gated
	// unless the calibrated return edge is absent or below cost.
	measurements := []*datura.Artifact{
		manifoldMeasurement("STRONG/USD", 0.8, 0.7, 0.1, 0.05),
		manifoldMeasurement("WEAK/USD", 0.8, 0.7, 0.1, 0.05),
		manifoldMeasurement("FLAT/USD", 0.8, 0.7, 0.1, 0.05),
		causalMeasurement("STRONG/USD", 2.0),
		causalMeasurement("WEAK/USD", 0.5),
		causalMeasurement("FLAT/USD", 0.0),
	}

	actions := []*datura.Artifact{
		candidate("WEAK/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("STRONG/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("FLAT/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, _ := decider.choose(measurements, actions, nil)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	if !containsSymbol(symbols, "FLAT/USD") {
		t.Fatalf("calibrated entry with flat causal uplift was gated: chosen=%v", symbols)
	}

	strongRank := indexOf(symbols, "STRONG/USD")
	weakRank := indexOf(symbols, "WEAK/USD")

	if strongRank < 0 || weakRank < 0 {
		t.Fatalf("a positive-uplift entry was dropped: chosen=%v", symbols)
	}

	if strongRank > weakRank {
		t.Fatalf("higher counterfactual uplift did not rank first: chosen=%v", symbols)
	}
}

func TestDeciderBlocksUnpricedEconomicCandidate(t *testing.T) {
	decider := testDecider()

	unready := causalMeasurement("EARLY/USD", 0)
	unready.MergeOutput("counterfactualReady", false)

	actions := []*datura.Artifact{
		candidate("EARLY/USD", logic.SideBuy, logic.ActionMarket, 0.6).
			Poke(0.0, "decision", "expected_return_bps").
			Poke(0, "decision", "sample_count"),
	}

	chosen, rejections := decider.choose([]*datura.Artifact{unready}, actions, nil)

	if len(chosen) != 0 {
		t.Fatalf("unpriced economic candidate was chosen: chosen=%d rejections=%v", len(chosen), rejections)
	}

	if len(rejections) != 1 || rejections[0].reason != "edge_unavailable" {
		t.Fatalf("unpriced economic verdict missing: %#v", rejections)
	}
}

func TestDeciderBlocksPositiveEdgeBelowRoundTripFriction(t *testing.T) {
	decider := testDecider()

	actions := []*datura.Artifact{
		candidate("DUST/USD", logic.SideBuy, logic.ActionMarket, 0.9).
			Poke(10.0, "decision", "expected_return_bps").
			Poke(30, "decision", "sample_count"),
	}

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("DUST/USD", 0.004)},
		actions,
		nil,
	)

	if len(chosen) != 0 {
		t.Fatalf("below-cost candidate was chosen: %d", len(chosen))
	}

	if len(verdicts) != 1 || verdicts[0].reason != "below_edge" {
		t.Fatalf("economic hurdle verdict missing: %#v", verdicts)
	}

	if edge := datura.Peek[float64](actions[0], "decision", "expected_return_bps"); edge != 10 {
		t.Fatalf("decision expected edge = %v, want 10", edge)
	}

	if hurdle := datura.Peek[float64](actions[0], "decision", "hurdle"); math.Abs(hurdle-0.0084) > 1e-12 {
		t.Fatalf("decision hurdle = %v, want 0.0084", hurdle)
	}
}

func TestEdgeEstimatorUsesConfiguredForwardHorizon(t *testing.T) {
	viper.Set("market.story.forward_return_min_samples", 2)
	viper.Set("trading.edge_min_bps", 10.0)
	viper.Set("trading.edge.forward_return_horizon", "5m")

	tree := dmt.NewTree("")
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	setupKey := "fluid|laminar|buy|market"
	insertExecutionFill(t, tree, "TREE/USD", "buy", 100, base, setupKey)
	insertTickerMark(t, tree, "TREE/USD", 200, base.Add(time.Second))
	insertExecutionFill(t, tree, "TREE/USD", "sell", 250, base.Add(2*time.Minute), setupKey)
	insertTickerMark(t, tree, "TREE/USD", 101, base.Add(5*time.Minute))
	insertExecutionFill(t, tree, "TREE/USD", "buy", 100, base.Add(10*time.Second), setupKey)
	insertTickerMark(t, tree, "TREE/USD", 102, base.Add(5*time.Minute+10*time.Second))

	decider := &Decider{
		economics: executionEconomics{
			takerFeeBps: 40,
			makerFeeBps: 25,
			slippageBps: 2,
		},
		tree: tree,
	}
	action := candidate("TREE/USD", logic.SideBuy, logic.ActionMarket, 0.8).
		Poke(0.0, "decision", "expected_return_bps").
		Poke(0, "decision", "sample_count").
		Poke(setupKey, "decision", "setup_key")

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("TREE/USD", 0.1)},
		[]*datura.Artifact{action},
		nil,
	)

	if len(chosen) != 1 {
		t.Fatalf("tree-calibrated entry should clear: chosen=%d verdicts=%v", len(chosen), verdicts)
	}
	if samples := datura.Peek[int](chosen[0], "decision", "sample_count"); samples != 2 {
		t.Fatalf("sample_count = %d, want 2", samples)
	}
	if source := datura.Peek[string](chosen[0], "decision", "edge_source"); source != "forward_return" {
		t.Fatalf("edge_source = %q, want forward_return", source)
	}
	if expected := datura.Peek[float64](chosen[0], "decision", "expected_return_bps"); math.Abs(expected-150) > 1e-9 {
		t.Fatalf("expected_return_bps = %v, want configured-horizon return 150", expected)
	}
	if key := datura.Peek[string](chosen[0], "decision", "edge_key"); key != setupKey {
		t.Fatalf("decision edge_key = %q, want %q", key, setupKey)
	}
}

func TestEdgeEstimatorUsesCandidateOutcomes(t *testing.T) {
	viper.Set("market.story.forward_return_min_samples", 2)
	viper.Set("trading.edge_min_bps", 10.0)
	viper.Set("trading.edge.forward_return_horizon", "5m")

	tree := dmt.NewTree("")
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	setupKey := "fluid|laminar|buy|market"
	insertCandidateOutcome(t, tree, "TREE/USD", "buy", setupKey, 0.015, base)
	insertCandidateOutcome(t, tree, "TREE/USD", "buy", setupKey, 0.020, base.Add(time.Minute))

	decider := &Decider{
		economics: executionEconomics{
			takerFeeBps: 40,
			makerFeeBps: 25,
			slippageBps: 2,
		},
		tree: tree,
	}
	action := candidate("TREE/USD", logic.SideBuy, logic.ActionMarket, 0.8).
		Poke(0.0, "decision", "expected_return_bps").
		Poke(0, "decision", "sample_count").
		Poke(setupKey, "decision", "setup_key")

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("TREE/USD", 0.1)},
		[]*datura.Artifact{action},
		nil,
	)

	if len(chosen) != 1 {
		t.Fatalf("candidate-outcome calibrated entry should clear: chosen=%d verdicts=%v", len(chosen), verdicts)
	}
	if samples := datura.Peek[int](chosen[0], "decision", "sample_count"); samples != 2 {
		t.Fatalf("sample_count = %d, want 2", samples)
	}
	if expected := datura.Peek[float64](chosen[0], "decision", "expected_return_bps"); math.Abs(expected-175) > 1e-9 {
		t.Fatalf("expected_return_bps = %v, want candidate outcome mean 175", expected)
	}
}

func TestEdgeEstimatorDoesNotMixRealizedReturnsIntoAdmission(t *testing.T) {
	viper.Set("market.story.forward_return_min_samples", 1)
	viper.Set("trading.edge_min_bps", 10.0)
	viper.Set("trading.edge.forward_return_horizon", "5m")

	tree := dmt.NewTree("")
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	setupKey := "fluid|laminar|buy|market"
	insertExecutionFill(t, tree, "TREE/USD", "buy", 100, base, setupKey)
	insertExecutionFill(t, tree, "TREE/USD", "sell", 200, base.Add(time.Minute), setupKey)

	decider := &Decider{
		economics: executionEconomics{
			takerFeeBps: 40,
			makerFeeBps: 25,
			slippageBps: 2,
		},
		tree: tree,
	}
	action := candidate("TREE/USD", logic.SideBuy, logic.ActionMarket, 0.8).
		Poke(0.0, "decision", "expected_return_bps").
		Poke(0, "decision", "sample_count").
		Poke(setupKey, "decision", "setup_key")

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("TREE/USD", 0.1)},
		[]*datura.Artifact{action},
		nil,
	)

	if len(chosen) != 0 {
		t.Fatalf("realized exit return should not clear entry admission: chosen=%d", len(chosen))
	}
	if len(verdicts) != 1 || verdicts[0].reason != "edge_unavailable" {
		t.Fatalf("realized-only edge verdict = %#v, want edge_unavailable", verdicts)
	}
}

func TestEdgeEstimatorRequiresFullSetupKey(t *testing.T) {
	viper.Set("market.story.forward_return_min_samples", 2)
	viper.Set("trading.edge_min_bps", 10.0)
	viper.Set("trading.edge.forward_return_horizon", "5m")

	tree := dmt.NewTree("")
	base := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	insertExecutionFill(t, tree, "TREE/USD", "buy", 100, base, "")
	insertExecutionFill(t, tree, "TREE/USD", "sell", 103, base.Add(time.Second), "")
	insertTickerMark(t, tree, "TREE/USD", 104, base.Add(2*time.Second))

	decider := &Decider{
		economics: executionEconomics{
			takerFeeBps: 40,
			makerFeeBps: 25,
			slippageBps: 2,
		},
		tree: tree,
	}
	action := candidate("TREE/USD", logic.SideBuy, logic.ActionMarket, 0.8).
		Poke(0.0, "decision", "expected_return_bps").
		Poke(0, "decision", "sample_count").
		Poke("", "decision", "setup_key")

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("TREE/USD", 0.1)},
		[]*datura.Artifact{action},
		nil,
	)

	if len(chosen) != 0 {
		t.Fatalf("symbol-only edge should not clear: chosen=%d", len(chosen))
	}
	if len(verdicts) != 1 || verdicts[0].reason != "edge_unavailable" {
		t.Fatalf("symbol-only edge verdict = %#v, want edge_unavailable", verdicts)
	}
}

func TestZeroExpectedReturnIsBelowEdgeNotUnavailable(t *testing.T) {
	decider := testDecider()
	action := candidate("ZERO/USD", logic.SideBuy, logic.ActionMarket, 0.8).
		Poke(0.0, "decision", "expected_return_bps").
		Poke(30, "decision", "sample_count")

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("ZERO/USD", 0.1)},
		[]*datura.Artifact{action},
		nil,
	)

	if len(chosen) != 0 {
		t.Fatalf("zero edge should not clear: chosen=%d", len(chosen))
	}
	if len(verdicts) != 1 || verdicts[0].reason != "below_edge" {
		t.Fatalf("zero edge verdict = %#v, want below_edge", verdicts)
	}
	if ready := datura.Peek[bool](action, "decision", "calibration_ready"); !ready {
		t.Fatalf("zero edge with enough samples should be calibration_ready")
	}
}

func TestDeciderPricesLimitEntryWithMakerHurdle(t *testing.T) {
	decider := testDecider()

	actions := []*datura.Artifact{
		candidate("COIL/USD", logic.SideBuy, logic.ActionLimit, 0.9),
	}

	chosen, verdicts := decider.choose(
		[]*datura.Artifact{causalMeasurement("COIL/USD", 0.007)},
		actions,
		nil,
	)

	if len(chosen) != 1 {
		t.Fatalf("maker-priced edge should clear limit hurdle: chosen=%d verdicts=%v", len(chosen), verdicts)
	}

	if hurdle := datura.Peek[float64](chosen[0], "decision", "hurdle"); math.Abs(hurdle-0.0069) > 1e-12 {
		t.Fatalf("decision hurdle = %v, want maker round-trip 0.0069", hurdle)
	}

	if liquidity := datura.Peek[string](chosen[0], "execution", "liquidity"); liquidity != "maker" {
		t.Fatalf("execution liquidity = %q, want maker", liquidity)
	}
}

func insertExecutionFill(
	t *testing.T,
	tree *dmt.Tree,
	symbol string,
	side string,
	price float64,
	stamp time.Time,
	setupKey string,
) {
	t.Helper()

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"channel": "executions",
			"type":    "update",
			"data": []datura.Map[any]{
				{
					"symbol":       symbol,
					"side":         side,
					"order_status": "filled",
					"avg_price":    price,
					"last_price":   price,
					"setup_key":    setupKey,
				},
			},
		}.Marshal())
	fill.SetTimestamp(stamp.UnixNano())
	tree.InsertArtifact(fill.Prefix("role", "timestamp"), fill)
}

func insertTickerMark(t *testing.T, tree *dmt.Tree, symbol string, price float64, stamp time.Time) {
	t.Helper()

	ticker := datura.Acquire("test", datura.APPJSON).
		WithRole("ticker").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"type":    "update",
			"data": []datura.Map[any]{
				{
					"symbol": symbol,
					"last":   price,
				},
			},
		}.Marshal())
	ticker.SetTimestamp(stamp.UnixNano())
	tree.InsertArtifact(ticker.Prefix("role", "timestamp"), ticker)
}

func insertCandidateOutcome(
	t *testing.T,
	tree *dmt.Tree,
	symbol string,
	side string,
	setupKey string,
	reward float64,
	stamp time.Time,
) {
	t.Helper()

	outcome := datura.Acquire("test", datura.APPJSON).
		WithRole("candidate_outcome").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"channel": "candidate_outcome",
			"type":    "update",
			"data": []datura.Map[any]{
				{
					"symbol":     symbol,
					"side":       side,
					"setup_key":  setupKey,
					"edge_key":   setupKey,
					"reward":     reward,
					"reward_bps": reward * 10_000,
				},
			},
		}.Marshal())
	outcome.SetTimestamp(stamp.UnixNano())
	tree.InsertArtifact(outcome.Prefix("role", "scope", "timestamp"), outcome)
}

func TestDeciderDoesNotBlockCoreCandidateWhenFieldSignalsAbsent(t *testing.T) {
	decider := testDecider()

	actions := []*datura.Artifact{
		candidate("CORE/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, rejections := decider.choose(
		[]*datura.Artifact{causalMeasurement("CORE/USD", 1.0)},
		actions,
		nil,
	)

	if len(chosen) != 1 {
		t.Fatalf("core playbook candidate was blocked: chosen=%d rejections=%v", len(chosen), rejections)
	}

	symbol, _ := chosen[0].Scope()
	if symbol != "CORE/USD" {
		t.Fatalf("chosen symbol=%q, want CORE/USD", symbol)
	}
}

func TestDeciderNormalizesMarketWideSurprise(t *testing.T) {
	decider := testDecider()

	measurements := []*datura.Artifact{
		causalMeasurement("A/USD", 1.0),
		causalMeasurement("B/USD", 1.0),
		resonanceMeasurement("A/USD", 5.0),
		resonanceMeasurement("B/USD", 5.0),
	}

	actions := []*datura.Artifact{
		candidate("A/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("B/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, verdicts := decider.choose(measurements, actions, nil)

	if len(chosen) != 2 {
		t.Fatalf("market-wide surprise should not zero candidates: chosen=%d verdicts=%v", len(chosen), verdicts)
	}

	for _, action := range chosen {
		if score := datura.Peek[float64](action, "decision", "score"); score <= 0.5 {
			t.Fatalf("market-wide surprise collapsed precision: score=%v", score)
		}
	}
}

func TestDeciderHoldingsGateRejectsHeldSymbols(t *testing.T) {
	decider := testDecider()

	// Two equally attractive entries; the ledger already holds BTC, so a fresh
	// entry on BTC/USD must be vetoed by holdings while ETH/USD survives.
	measurements := []*datura.Artifact{
		manifoldMeasurement("BTC/USD", 0.8, 0.7, 0.1, 0.05),
		manifoldMeasurement("ETH/USD", 0.8, 0.7, 0.1, 0.05),
		causalMeasurement("BTC/USD", 1.0),
		causalMeasurement("ETH/USD", 1.0),
	}

	actions := []*datura.Artifact{
		candidate("BTC/USD", logic.SideBuy, logic.ActionMarket, 0.6),
		candidate("ETH/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	var balances *datura.Artifact
	for artifact := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
		balances = artifact
		break
	}

	chosen, _ := decider.choose(measurements, actions, balances)

	symbols := make([]string, 0, len(chosen))

	for _, action := range chosen {
		symbol, _ := action.Scope()
		symbols = append(symbols, symbol)
	}

	if containsSymbol(symbols, "BTC/USD") {
		t.Fatalf("entry on a held symbol was not gated: chosen=%v", symbols)
	}

	if !containsSymbol(symbols, "ETH/USD") {
		t.Fatalf("entry on an unheld symbol was dropped: chosen=%v", symbols)
	}
}

func TestDeciderIgnoresEmptyBalanceArtifact(t *testing.T) {
	decider := testDecider()
	action := candidate("CORE/USD", logic.SideBuy, logic.ActionMarket, 0.6)
	balances := datura.Acquire("test", datura.APPJSON).WithRole("balances")

	chosen, rejections := decider.choose(
		[]*datura.Artifact{causalMeasurement("CORE/USD", 1.0)},
		[]*datura.Artifact{action},
		balances,
	)

	if len(chosen) != 1 {
		t.Fatalf("empty balances should not block fresh candidate: chosen=%d rejections=%v", len(chosen), rejections)
	}
}

func TestDeciderEmptyActions(t *testing.T) {
	decider := testDecider()

	if chosen, _ := decider.choose(nil, nil, nil); len(chosen) != 0 {
		t.Fatalf("expected no actions, got %d", len(chosen))
	}
}

func TestDeciderStampsVerdictOnCandidateArtifact(t *testing.T) {
	decider := testDecider()
	action := candidate("CORE/USD", logic.SideBuy, logic.ActionMarket, 0)

	chosen, verdicts := decider.choose(nil, []*datura.Artifact{action}, nil)

	if len(chosen) != 0 {
		t.Fatalf("candidate without entry confidence was chosen: %d", len(chosen))
	}

	if len(verdicts) != 1 || verdicts[0].action != action {
		t.Fatalf("candidate verdict not returned: %#v", verdicts)
	}

	if got := datura.Peek[string](action, "verdict"); got != "blocked" {
		t.Fatalf("verdict attribute = %q, want blocked", got)
	}

	if got := datura.Peek[string](action, "journey", "trader", "reason"); got != "no_entry_confidence" {
		t.Fatalf("journey reason = %q, want no_entry_confidence", got)
	}
}

func containsSymbol(symbols []string, target string) bool {
	return indexOf(symbols, target) >= 0
}

func indexOf(symbols []string, target string) int {
	for index, symbol := range symbols {
		if symbol == target {
			return index
		}
	}

	return -1
}
