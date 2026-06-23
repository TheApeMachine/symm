package trader

import (
	"encoding/json"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestCryptoDecisionTracePublishesBackendCandidates(t *testing.T) {
	crypto := &Crypto{}
	trace := crypto.publishDecisionTrace([]*datura.Artifact{
		traderMeasurement(logic.SourcePumpDump, "NEAR/EUR", 0.6),
		traderMeasurement(logic.SourceCausal, "NEAR/EUR", 0.4),
		traderMeasurement(logic.SourceLiquidity, "PEPE/EUR", 0.9),
		traderMeasurement(logic.SourceHawkes, "ATOM/EUR", 0.3),
	})

	if trace == nil {
		t.Fatal("publishDecisionTrace returned nil")
	}

	role, _ := trace.Role()

	if role != "decision_trace" {
		t.Fatalf("role = %q, want decision_trace", role)
	}

	var frame decisionTraceFrame

	if err := json.Unmarshal(trace.DecryptPayload(), &frame); err != nil {
		t.Fatal(err)
	}

	if frame.Type != "decision_trace" {
		t.Fatalf("type = %q, want decision_trace", frame.Type)
	}

	if frame.StoryTicks != 1 {
		t.Fatalf("story ticks = %d, want 1", frame.StoryTicks)
	}

	if len(frame.Decisions) != 3 {
		t.Fatalf("decisions = %d, want 3", len(frame.Decisions))
	}

	if frame.Decisions[0].Symbol != "PEPE/EUR" {
		t.Fatalf("top symbol = %q, want PEPE/EUR", frame.Decisions[0].Symbol)
	}

	if !frame.Decisions[0].InPlay {
		t.Fatal("top decision should be in play")
	}

	near := frame.Decisions[1]

	if near.Symbol != "NEAR/EUR" || len(near.Signals) != 2 {
		t.Fatalf("NEAR row = %+v, want two signal rows", near)
	}
}

func BenchmarkCryptoDecisionTrace(benchmark *testing.B) {
	benchmark.ReportAllocs()

	measurements := []*datura.Artifact{
		traderMeasurement(logic.SourcePumpDump, "NEAR/EUR", 0.6),
		traderMeasurement(logic.SourceCausal, "NEAR/EUR", 0.4),
		traderMeasurement(logic.SourceLiquidity, "PEPE/EUR", 0.9),
		traderMeasurement(logic.SourceHawkes, "ATOM/EUR", 0.3),
	}
	crypto := &Crypto{}

	for benchmark.Loop() {
		if crypto.publishDecisionTrace(measurements) == nil {
			benchmark.Fatal("publishDecisionTrace returned nil")
		}
	}
}

func traderMeasurement(
	source logic.SourceType,
	symbol string,
	confidence float64,
) *datura.Artifact {
	payload, _ := json.Marshal(map[string]any{
		"data": []map[string]any{
			{"symbol": symbol},
		},
		"output": map[string]any{
			"confidence": confidence,
		},
	})

	return datura.Acquire(string(source), datura.APPJSON).
		WithRole("measurement").
		WithScope("update").
		WithPayload(payload)
}
