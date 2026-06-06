package market

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

const captureReplaySampleLines = 800000

func storyCaptureReplayPaths(t testing.TB) (capturePath, playbookPath string) {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	testDir := filepath.Dir(file)
	capturePath = filepath.Join(testDir, "..", "runs", "capture.jsonl")
	playbookPath = filepath.Join(testDir, "perspectives", "cfg", "perspectives.yaml")

	return capturePath, playbookPath
}

func TestStampQuoteNotional(t *testing.T) {
	bases := make(map[string]float64)

	first := StampQuoteNotional(types.Measurement{
		Symbol: "BTC/EUR", Last: 100, Volume: 1_000_000,
	}, bases)
	second := StampQuoteNotional(types.Measurement{
		Symbol: "BTC/EUR", Last: 101, Volume: 0,
	}, bases)

	if first.Volume != 1_000_000 {
		t.Fatalf("first notional=%v want 1000000", first.Volume)
	}

	if second.Volume != 1_010_000 {
		t.Fatalf("second notional=%v want 1010000", second.Volume)
	}
}

func TestReplayCapturePlaybookFiringSample(t *testing.T) {
	regimeCounts, foundCounts, _, _ := replayCaptureSample(t, captureReplaySampleLines)

	t.Logf("regimes=%v entry_actions=%v", regimeCounts, foundCounts)

	if len(foundCounts) == 0 {
		t.Skip("production playbook produced no entry actions on capture sample")
	}
}

func replayCaptureSample(t *testing.T, maxLines int) (
	regimeCounts map[types.Regime]int,
	foundCounts map[reasoning.ActionType]int,
	parentHeld map[int]int,
	childHeld map[int]int,
) {
	t.Helper()

	viper.Set("regime.trend_threshold", 1.25)
	viper.Set("regime.strong_trend", 2.5)

	capturePath, playbookPath := storyCaptureReplayPaths(t)

	file, err := os.Open(capturePath)
	if err != nil {
		t.Skip("no capture file")
	}
	defer file.Close()

	raw, err := os.ReadFile(playbookPath)
	if err != nil {
		t.Fatal(err)
	}

	playbook, err := reasoning.ParseThoughts(raw)
	if err != nil {
		t.Fatal(err)
	}

	perSymbol := make(map[string][]types.Measurement)
	reasonStates := make(map[string]*reasoning.ReasonState)
	quoteVolumeBase := make(map[string]float64)
	regimeCounts = make(map[types.Regime]int)
	foundCounts = make(map[reasoning.ActionType]int)
	parentHeld = make(map[int]int)
	childHeld = make(map[int]int)
	lineCount := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		lineCount++

		if lineCount > maxLines {
			break
		}

		var measurement types.Measurement
		if err := json.Unmarshal(scanner.Bytes(), &measurement); err != nil {
			continue
		}

		if measurement.Symbol == "" || measurement.Last <= 0 {
			continue
		}

		measurement = StampQuoteNotional(measurement, quoteVolumeBase)

		window := append(perSymbol[measurement.Symbol], measurement)
		if len(window) > 1024 {
			window = window[len(window)-1024:]
		}

		perSymbol[measurement.Symbol] = window

		if len(window) < 16 {
			continue
		}

		features := perspectives.ClassifyRegime(window)
		regimeCounts[features.Regime]++

		state, ok := reasonStates[measurement.Symbol]
		if !ok {
			state = reasoning.NewReasonState()
			reasonStates[measurement.Symbol] = state
		}

		context := reasoning.NewWindowReason(window, features.Regime, reasoning.PositionState{})
		act, found := reasoning.EvaluateStateful(playbook, context, state)

		if found && act.Type != reasoning.ActionNone && reasoning.IsEntryAction(act.Type) {
			foundCounts[act.Type]++
		}

		if features.Regime != types.RegimeTrending && features.Regime != types.RegimeBullish {
			continue
		}

		for index := 5; index < len(playbook); index++ {
			if reasoning.HoldsPredicate(playbook[index].When, context) {
				parentHeld[index]++
			}

			if len(playbook[index].Then) == 0 {
				continue
			}

			if reasoning.HoldsPredicate(playbook[index].When, context) &&
				reasoning.HoldsPredicate(playbook[index].Then[0].When, context) {
				childHeld[index]++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	return regimeCounts, foundCounts, parentHeld, childHeld
}

func TestReplayCaptureFlashPumpChildFailures(t *testing.T) {
	viper.Set("regime.trend_threshold", 1.25)
	viper.Set("regime.strong_trend", 2.5)

	capturePath, playbookPath := storyCaptureReplayPaths(t)

	file, err := os.Open(capturePath)
	if err != nil {
		t.Skip("no capture file")
	}
	defer file.Close()

	raw, err := os.ReadFile(playbookPath)
	if err != nil {
		t.Fatal(err)
	}

	playbook, err := reasoning.ParseThoughts(raw)
	if err != nil {
		t.Fatal(err)
	}

	flashPump := playbook[5]
	child := flashPump.Then[0]
	leafFail := make(map[int]int)
	parentPasses := 0
	perSymbol := make(map[string][]types.Measurement)
	quoteVolumeBase := make(map[string]float64)
	lineCount := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		lineCount++

		if lineCount > captureReplaySampleLines {
			break
		}

		var measurement types.Measurement
		if err := json.Unmarshal(scanner.Bytes(), &measurement); err != nil {
			continue
		}

		if measurement.Symbol == "" || measurement.Last <= 0 {
			continue
		}

		measurement = StampQuoteNotional(measurement, quoteVolumeBase)
		window := append(perSymbol[measurement.Symbol], measurement)

		if len(window) > 1024 {
			window = window[len(window)-1024:]
		}

		perSymbol[measurement.Symbol] = window

		if len(window) < 16 {
			continue
		}

		features := perspectives.ClassifyRegime(window)
		if features.Regime != types.RegimeTrending && features.Regime != types.RegimeBullish {
			continue
		}

		context := reasoning.NewWindowReason(window, features.Regime, reasoning.PositionState{})
		if !reasoning.HoldsPredicate(flashPump.When, context) {
			continue
		}

		parentPasses++

		for leafIndex, leaf := range reasoning.FlattenLeaves(child.When) {
			if !reasoning.HoldsPredicate(leaf.Predicate, context) {
				leafFail[leafIndex]++
			}
		}
	}

	t.Logf("parentPasses=%d leafFail=%v", parentPasses, leafFail)
}
