package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/theapemachine/symm/types"
)

func TestFlushRemovesWithoutRecorder(t *testing.T) {
	stager := NewStager(nil)
	decision := &types.Decision{ID: "decision-1", Symbol: "BTC/USD"}
	stager.Stage(decision, time.Hour)

	if err := stager.Flush(decision.ID); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if pending := stager.Pending(); len(pending) != 0 {
		t.Fatalf("pending decisions after flush = %d, want 0", len(pending))
	}
}

func TestFlushWritesStableDecisionBatch(t *testing.T) {
	var kind string
	var payload []byte
	recorder := &Recorder{EventSink: func(gotKind string, gotPayload []byte) error {
		kind = gotKind
		payload = append([]byte(nil), gotPayload...)
		return nil
	}}
	stager := NewStager(recorder)
	decision := &types.Decision{ID: "decision-2", Symbol: "ETH/USD"}
	stager.Stage(decision, time.Hour)

	if err := stager.Flush(decision.ID); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if kind != DecisionBatchEvent {
		t.Fatalf("event kind = %q, want %q", kind, DecisionBatchEvent)
	}
	var decisions []types.Decision
	if err := json.Unmarshal(payload, &decisions); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(decisions) != 1 || decisions[0].ID != decision.ID {
		t.Fatalf("decoded decisions = %#v", decisions)
	}
}
