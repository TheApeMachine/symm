package causal

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestGraphSnapshotReportsPearlMetrics(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "ALT/EUR"}, engine.DefaultCalibrationParams())

	for index := 0; index < minCausalHistory; index++ {
		state.samples = append(state.samples, causalSample{
			macroMomentum: float64(index%4) * 0.005,
			liquidity:     1 + float64(index%3)*0.15,
			localFlow:     float64(index) * 0.4,
			priceVelocity: float64(index) * 0.08,
		})
	}

	state.lastPrice = 1
	state.buyPressure = 1
	state.imbalance = 0.8
	state.spreadBPS = 12
	state.hasPrior = true
	state.lastAt = time.Now().Add(-time.Second)

	payload := state.GraphSnapshot(0.015, time.Now())

	if payload["event"] != "causal_graph" {
		t.Fatalf("expected causal_graph event, got %v", payload["event"])
	}

	if ready, ok := payload["ready"].(bool); !ok || !ready {
		t.Fatalf("expected ready graph snapshot, got %v", payload["ready"])
	}

	intervention, ok := payload["intervention"].(float64)

	if !ok || intervention <= 0 {
		t.Fatalf("expected positive intervention, got %v", payload["intervention"])
	}
}

func TestGraphSnapshotColdHistory(t *testing.T) {
	state := NewCausalSymbol(asset.Pair{Wsname: "ALT/EUR"}, engine.DefaultCalibrationParams())
	payload := state.GraphSnapshot(0, time.Now())

	if ready, ok := payload["ready"].(bool); !ok || ready {
		t.Fatalf("expected cold snapshot, got %v", payload["ready"])
	}
}
