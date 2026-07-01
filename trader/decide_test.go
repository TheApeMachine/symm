package trader

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
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

	return action
}

func TestDeciderRanksByFieldEdgeAndGatesRisk(t *testing.T) {
	decider := NewDecider()

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
	measurement.Merge("surprise", surprise)

	return measurement
}

func TestDeciderResonanceSurpriseLowersPrecision(t *testing.T) {
	decider := NewDecider()

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
	decider := NewDecider()

	// Identical field and confidence on all three; the causal counterfactual is
	// the only difference. STRONG > WEAK by uplift; FLAT (no causal edge) is
	// gated out entirely — the field cannot manufacture an opportunity the
	// counterfactual does not see.
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

	if containsSymbol(symbols, "FLAT/USD") {
		t.Fatalf("entry with no causal edge was not gated: chosen=%v", symbols)
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

func TestDeciderIgnoresUnreadyCausalCounterfactual(t *testing.T) {
	decider := NewDecider()

	unready := causalMeasurement("EARLY/USD", 0)
	unready.MergeOutput("counterfactualReady", false)

	actions := []*datura.Artifact{
		candidate("EARLY/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, rejections := decider.choose([]*datura.Artifact{unready}, actions, nil)

	if len(chosen) != 1 {
		t.Fatalf("unready causal model blocked core candidate: chosen=%d rejections=%v", len(chosen), rejections)
	}
}

func TestDeciderDoesNotBlockCoreCandidateWhenFieldSignalsAbsent(t *testing.T) {
	decider := NewDecider()

	actions := []*datura.Artifact{
		candidate("CORE/USD", logic.SideBuy, logic.ActionMarket, 0.6),
	}

	chosen, rejections := decider.choose(nil, actions, nil)

	if len(chosen) != 1 {
		t.Fatalf("core playbook candidate was blocked: chosen=%d rejections=%v", len(chosen), rejections)
	}

	symbol, _ := chosen[0].Scope()
	if symbol != "CORE/USD" {
		t.Fatalf("chosen symbol=%q, want CORE/USD", symbol)
	}
}

func TestDeciderHoldingsGateRejectsHeldSymbols(t *testing.T) {
	decider := NewDecider()

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

func TestDeciderEmptyActions(t *testing.T) {
	decider := NewDecider()

	if chosen, _ := decider.choose(nil, nil, nil); len(chosen) != 0 {
		t.Fatalf("expected no actions, got %d", len(chosen))
	}
}

func TestDeciderStampsVerdictOnCandidateArtifact(t *testing.T) {
	decider := NewDecider()
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

	if got := datura.Peek[string](action, "journey", "trader", "reason"); got != "no entry confidence" {
		t.Fatalf("journey reason = %q, want no entry confidence", got)
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