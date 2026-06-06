package reasoning_test

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func loadProductionPlaybook(t *testing.T) []reasoning.Thought {
	t.Helper()

	raw, err := os.ReadFile("../cfg/perspectives.yaml")
	if err != nil {
		t.Fatalf("read production playbook: %v", err)
	}

	thoughts, err := reasoning.ParseThoughts(raw)
	if err != nil {
		t.Fatalf("parse production playbook: %v", err)
	}

	return thoughts
}

func TestReplayCapturePlaybookFiring(t *testing.T) {
	file, err := os.Open("../../runs/capture.jsonl")
	if err != nil {
		t.Skip("no capture file")
	}
	defer file.Close()

	playbook := loadProductionPlaybook(t)
	perSymbol := make(map[string][]types.Measurement)
	regimeCounts := make(map[types.Regime]int)
	foundCounts := make(map[reasoning.ActionType]int)
	evalCount := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var measurement types.Measurement
		if err := json.Unmarshal(scanner.Bytes(), &measurement); err != nil {
			continue
		}

		if measurement.Symbol == "" || measurement.Last <= 0 {
			continue
		}

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

		context := reasoning.NewWindowReason(window, features.Regime, reasoning.PositionState{})
		act, found := reasoning.Evaluate(playbook, context)
		evalCount++

		if found && act.Type != reasoning.ActionNone {
			foundCounts[act.Type]++
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	t.Logf("evaluations=%d regimes=%v actions=%v", evalCount, regimeCounts, foundCounts)

	if len(foundCounts) == 0 {
		t.Fatal("production playbook never fired on capture data")
	}
}

func TestReplayCaptureTrendingEntryGates(t *testing.T) {
	file, err := os.Open("../../runs/capture.jsonl")
	if err != nil {
		t.Skip("no capture file")
	}
	defer file.Close()

	playbook := loadProductionPlaybook(t)
	perSymbol := make(map[string][]types.Measurement)
	parentHeld := make(map[int]int)
	childHeld := make(map[int]int)
	trendingEvals := 0

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var measurement types.Measurement
		if err := json.Unmarshal(scanner.Bytes(), &measurement); err != nil {
			continue
		}

		if measurement.Symbol == "" || measurement.Last <= 0 {
			continue
		}

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

		trendingEvals++
		context := reasoning.NewWindowReason(window, features.Regime, reasoning.PositionState{})

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

	t.Logf("trending/bullish evals=%d parentHeld=%v childHeld=%v", trendingEvals, parentHeld, childHeld)
}
