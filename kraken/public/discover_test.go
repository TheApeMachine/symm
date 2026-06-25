package public

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

// TestRecordPairsPersistsAndSubscribes verifies the instrument frame is the
// symbol source of truth: online pairs are recorded to the tree per symbol with
// their full metadata, a snapshot (re)subscribes every pair, and an update only
// surfaces genuinely new ones.
func TestRecordPairsPersistsAndSubscribes(t *testing.T) {
	ws := &WebSocket{tree: dmt.NewTree("")}

	snapshot := []byte(`{"pairs":[
		{"symbol":"BTC/USD","status":"online","tick_size":0.1,"qty_min":0.0001},
		{"symbol":"ETH/USD","status":"online","tick_size":0.01,"qty_min":0.001},
		{"symbol":"DEAD/USD","status":"delisted"},
		{"symbol":"","status":"online"}
	]}`)

	fresh, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}

	if len(fresh) != 2 {
		t.Fatalf("want 2 online pairs from snapshot, got %d: %v", len(fresh), fresh)
	}

	// The full per-pair metadata must be queryable from the tree by symbol.
	raw, ok := ws.tree.Get(instrumentKey("BTC/USD"))
	if !ok {
		t.Fatal("BTC/USD instrument metadata not stored in tree")
	}

	artifact := datura.Acquire("test", datura.APPJSON)
	if _, err := artifact.Write(raw); err != nil {
		t.Fatalf("failed to read stored instrument artifact: %v", err)
	}

	var meta struct {
		TickSize float64 `json:"tick_size"`
		QtyMin   float64 `json:"qty_min"`
	}
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &meta); err != nil {
		t.Fatalf("failed to parse stored metadata: %v", err)
	}
	if meta.TickSize != 0.1 || meta.QtyMin != 0.0001 {
		t.Fatalf("metadata not preserved: tick_size=%v qty_min=%v", meta.TickSize, meta.QtyMin)
	}

	// An update re-reporting BTC/USD plus a new SOL/USD should surface only the
	// newcomer for subscription (BTC is already in the tree).
	update := []byte(`{"pairs":[
		{"symbol":"BTC/USD","status":"online","tick_size":0.1},
		{"symbol":"SOL/USD","status":"online","tick_size":0.001}
	]}`)

	again, err := ws.recordPairs(update, "update")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}
	if len(again) != 1 || again[0] != "SOL/USD" {
		t.Fatalf("want only fresh SOL/USD from update, got %v", again)
	}

	// A fresh snapshot (as on reconnect) re-subscribes every online pair even
	// though they are already in the tree.
	reconnect, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}
	if len(reconnect) != 2 {
		t.Fatalf("snapshot should re-subscribe all online pairs, got %v", reconnect)
	}
}
