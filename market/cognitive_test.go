package market

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
)

func cognitiveMeasurement(symbol, origin string, categoryIndex int, confidence float64) *datura.Artifact {
	measurement := datura.Acquire("test", datura.APPJSON)
	measurement.WithScope(symbol)
	_ = measurement.SetOrigin(origin)
	measurement.MergeOutput("category", float64(categoryIndex))
	measurement.MergeOutput("confidence", confidence)
	measurement.MergeOutput("strength", confidence)

	return measurement
}

func cognitiveStateSeed(symbol, origin string) *datura.Artifact {
	measurement := datura.Acquire("test", datura.APPJSON)
	measurement.WithScope(symbol)
	_ = measurement.SetOrigin(origin)
	measurement.Merge("kind", "trade")

	return measurement
}

/*
TestCognitiveReadingsUseEngine confirms the readings are produced by the dmt.Tree
cognitive engine: a sequence is encoded, the engine is trained on it, and the
reading carries the engine's outputs. It also confirms training mutates the shared
tree (sensory weights exist after the call) — i.e. the engine actually learns,
rather than the reading being invented from category masses.
*/
func TestCognitiveReadingsUseEngine(t *testing.T) {
	tree := dmt.NewTree("")

	measurements := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), 1, 0.9),
		cognitiveMeasurement("BTC/USD", string(logic.SourceFluid), 2, 0.4),
		cognitiveMeasurement("BTC/USD", string(logic.SourceHawkes), 3, 0.2),
	}

	readings := CognitiveReadings(tree, measurements)
	btc, ok := readings["BTC/USD"]

	if !ok {
		t.Fatalf("expected a reading for BTC/USD, got %v", readings)
	}

	if btc.Sequence == "" {
		t.Fatalf("reading must carry the encoded sensory sequence")
	}

	if btc.RegimeCohort != 3 {
		t.Fatalf("regime cohort should count contributing signals: got %d", btc.RegimeCohort)
	}

	if btc.WinnerClass == "" {
		t.Fatalf("reading must carry a winner class (engine winner or nascent regime)")
	}

	// The engine must have been trained: the leading prefix token now has a
	// sensory weight in the shared tree. This is the proof the engine learned,
	// not that we fabricated a number.
	leadToken := []byte(btc.Sequence)
	for index := range leadToken {
		if leadToken[index] == '_' {
			leadToken = leadToken[:index]

			break
		}
	}

	if tree.GetSensoryWeight(leadToken).Count == 0 {
		t.Fatalf("training did not write a sensory weight for the lead token %q", string(leadToken))
	}

	if len(btc.Branches) < 2 {
		t.Fatalf("reading must expose the trained sensory prefix branches, got %+v", btc.Branches)
	}
}

/*
TestCognitiveReadingsLearnOverTime proves the engine actually learns: feeding the
same regime repeatedly must make its attractor basin classifiable, so a later
read carries a positive class confidence where the first read (cold tree) could
not classify. This is the difference between reading a real engine and fabricating
a number — a fabricated reading would be identical every tick.
*/
func TestCognitiveReadingsLearnOverTime(t *testing.T) {
	tree := dmt.NewTree("")

	regime := func() []*datura.Artifact {
		return []*datura.Artifact{
			cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), 1, 0.9),
			cognitiveMeasurement("BTC/USD", string(logic.SourceFluid), 2, 0.5),
			cognitiveMeasurement("BTC/USD", string(logic.SourceHawkes), 3, 0.3),
		}
	}

	first := CognitiveReadings(tree, regime())["BTC/USD"]

	// Feed the same regime several more times; the basin should reinforce.
	var latest CognitiveReading
	for round := 0; round < 8; round++ {
		latest = CognitiveReadings(tree, regime())["BTC/USD"]
	}

	if first.ClassConfidence > 0 {
		t.Fatalf("cold tree should not classify on the first read: got %.3f", first.ClassConfidence)
	}

	if latest.ClassConfidence <= 0 {
		t.Fatalf("engine did not learn: class confidence still zero after repeated training")
	}

	// Predictive-coding signature: a novel regime is surprising; a familiar one is
	// not. After repeated exposure the same sequence must carry less surprise.
	if latest.EntropyBits >= first.EntropyBits {
		t.Fatalf(
			"entropy did not fall with familiarity: first=%.3f latest=%.3f",
			first.EntropyBits,
			latest.EntropyBits,
		)
	}

	if first.EntropyBits <= 0 {
		t.Fatalf("a novel regime must carry positive surprise: got %.3f", first.EntropyBits)
	}

	if len(latest.Beams) == 0 {
		t.Fatalf("engine beam search did not expose any DMT lookahead paths")
	}

	if len(latest.Classes) == 0 {
		t.Fatalf("engine classification did not expose posterior classes")
	}
}

/*
TestCognitiveReadingsNilTree guards the nil-tree contract: with no engine there is
nothing to read, so no readings (rather than a panic or fabricated output).
*/
func TestCognitiveReadingsNilTree(t *testing.T) {
	measurements := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), 1, 0.9),
	}

	if readings := CognitiveReadings(nil, measurements); readings != nil {
		t.Fatalf("nil tree must yield no readings, got %v", readings)
	}
}

/*
TestCognitiveReadingsPerSymbol confirms readings are grouped per symbol.
*/
func TestCognitiveReadingsPerSymbol(t *testing.T) {
	tree := dmt.NewTree("")

	measurements := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), 1, 0.8),
		cognitiveMeasurement("ETH/USD", string(logic.SourceFluid), 2, 0.6),
	}

	readings := CognitiveReadings(tree, measurements)

	if len(readings) != 2 {
		t.Fatalf("expected one reading per symbol, got %d", len(readings))
	}

	if readings["BTC/USD"].Scope != "BTC/USD" || readings["ETH/USD"].Scope != "ETH/USD" {
		t.Fatalf("readings must carry their own scope: %+v", readings)
	}
}

/*
TestCognitiveReadingsUseClassifiedMeasurementsOnly guards the live Cortex path:
state-seed artifacts are real measurement rows for replay, but they are not
classified regimes. They must not default to category 0 or inflate the cohort.
If an origin emits more than one classified artifact in a batch, the strongest
one is the current observation for that origin.
*/
func TestCognitiveReadingsUseClassifiedMeasurementsOnly(t *testing.T) {
	tree := dmt.NewTree("")

	measurements := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), logic.CategoryIndex(logic.CategoryCoiledCompression), 0.4),
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), logic.CategoryIndex(logic.CategoryVerticalIgnition), 0.8),
		cognitiveMeasurement("BTC/USD", string(logic.SourceToxicity), logic.CategoryIndex(logic.CategoryHardSupport), 0.7),
		cognitiveStateSeed("BTC/USD", string(logic.SourceDepthFlow)),
	}

	reading := CognitiveReadings(tree, measurements)["BTC/USD"]

	if reading.RegimeCohort != 2 {
		t.Fatalf("expected one classified token per origin, got cohort %d in %q", reading.RegimeCohort, reading.Sequence)
	}

	if reading.Sequence != "vertical-ignition_hard-support" {
		t.Fatalf("unexpected cognitive sequence: %q", reading.Sequence)
	}

	if strings.Contains(reading.Sequence, "none") || strings.Contains(reading.Sequence, "book-thinning") {
		t.Fatalf("state seed leaked into cognitive sequence: %q", reading.Sequence)
	}
}

func TestApplyCognitiveReadingsStampsMeasurementSurprise(t *testing.T) {
	tree := dmt.NewTree("")

	measurements := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), logic.CategoryIndex(logic.CategoryVerticalIgnition), 0.8),
		cognitiveMeasurement("ETH/USD", string(logic.SourceFluid), logic.CategoryIndex(logic.CategoryLaminar), 0.6),
	}

	readings := CognitiveReadings(tree, measurements)
	ApplyCognitiveReadings(measurements, readings)

	for _, measurement := range measurements {
		symbol, _ := measurement.Scope()
		reading := readings[symbol]
		gotSurprisal := datura.Peek[float64](measurement, "output", "surprisal")
		gotSurprise := datura.Peek[float64](measurement, "output", "surprise")
		gotStatus := datura.Peek[string](measurement, "output", "status")

		if gotSurprisal != reading.Surprisal {
			t.Fatalf("surprisal for %s was not stamped from DMT reading: got %.3f want %.3f", symbol, gotSurprisal, reading.Surprisal)
		}

		if gotSurprise != reading.Surprise {
			t.Fatalf("surprise ratio for %s was not stamped from DMT reading: got %.3f want %.3f", symbol, gotSurprise, reading.Surprise)
		}

		if gotStatus == "" {
			t.Fatalf("backend status was not stamped for %s", symbol)
		}
	}
}

func TestCognitiveEvaluatorUsesCachedReadingsWhenBudgetIsExhausted(t *testing.T) {
	tree := dmt.NewTree("")
	evaluator := NewCognitiveEvaluator(tree)

	warm := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), logic.CategoryIndex(logic.CategoryVerticalIgnition), 0.8),
	}
	initial := evaluator.Readings(warm, time.Second)

	if initial["BTC/USD"].Sequence == "" {
		t.Fatalf("warm read did not compute a BTC/USD cognitive reading")
	}

	current := []*datura.Artifact{
		cognitiveMeasurement("BTC/USD", string(logic.SourcePumpDump), logic.CategoryIndex(logic.CategoryVerticalIgnition), 0.8),
		cognitiveMeasurement("ETH/USD", string(logic.SourceFluid), logic.CategoryIndex(logic.CategoryLaminar), 0.6),
	}
	readings := evaluator.Readings(current, 0)

	if readings["BTC/USD"].Sequence != initial["BTC/USD"].Sequence {
		t.Fatalf("expected cached BTC/USD reading under zero budget, got %+v", readings["BTC/USD"])
	}

	if _, exists := readings["ETH/USD"]; exists {
		t.Fatalf("uncached ETH/USD should not be invented when budget is exhausted")
	}

	ApplyCognitiveReadings(current, readings)
	if got := datura.Peek[float64](current[0], "output", "surprisal"); got != initial["BTC/USD"].Surprisal {
		t.Fatalf("cached surprisal was not stamped: got %.3f want %.3f", got, initial["BTC/USD"].Surprisal)
	}
	if got := datura.Peek[float64](current[1], "output", "surprisal"); got != 0 {
		t.Fatalf("uncached ETH/USD should not receive fabricated surprisal: got %.3f", got)
	}
}

func BenchmarkCognitiveReadingsQuoteBoard(benchmark *testing.B) {
	measurements := make([]*datura.Artifact, 0, 397*5)
	origins := []logic.SourceType{
		logic.SourceCausal,
		logic.SourceCVD,
		logic.SourceLiquidity,
		logic.SourceSentiment,
		logic.SourceToxicity,
	}
	categories := []logic.CategoryType{
		logic.CategoryEndogenousAlpha,
		logic.CategoryHiddenAbsorption,
		logic.CategoryRobustLiquidity,
		logic.CategoryRiskOnSurge,
		logic.CategoryHardSupport,
	}

	for symbolIndex := range 397 {
		symbol := fmt.Sprintf("SYM%03d/USD", symbolIndex)

		for originIndex, origin := range origins {
			measurements = append(measurements, cognitiveMeasurement(
				symbol,
				string(origin),
				logic.CategoryIndex(categories[originIndex]),
				0.5+float64(originIndex)*0.05,
			))
		}
	}

	tree := dmt.NewTree("")

	for range 3 {
		CognitiveReadings(tree, measurements)
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		CognitiveReadings(tree, measurements)
	}
}
