package market

import (
	"testing"

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
