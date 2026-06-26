package public

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken/types"
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
	if _, err := artifact.Unpack(raw); err != nil {
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

func TestRecordPairsSubscribesAllOnlinePairsRegardlessOfQuote(t *testing.T) {
	ws := &WebSocket{tree: dmt.NewTree("")}

	snapshot := []byte(`{"pairs":[
		{"symbol":"BTC/USD","status":"online","tick_size":0.1},
		{"symbol":"ETH/EUR","status":"online","tick_size":0.01}
	]}`)

	fresh, err := ws.recordPairs(snapshot, "snapshot")
	if err != nil {
		t.Fatalf("recordPairs returned error: %v", err)
	}

	if len(fresh) != 2 || fresh[0] != "BTC/USD" || fresh[1] != "ETH/EUR" {
		t.Fatalf("want every online pair subscribed, got %v", fresh)
	}

	if _, ok := ws.tree.Get(instrumentKey("ETH/EUR")); !ok {
		t.Fatal("cross-quote instrument metadata was not stored")
	}
}

func TestPersistMarketFrameScopesRowsBySymbol(t *testing.T) {
	ws := &WebSocket{tree: dmt.NewTree("")}

	ws.persistMarketFrame(types.SocketMessage{
		Channel: "ticker",
		Type:    "update",
		Data: []byte(`[
			{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":10},
			{"symbol":"ETH/EUR","bid":199,"ask":201,"last":200,"volume":20}
		]`),
	})

	btc := firstArtifact(ws.tree.Seek([]byte("ticker/BTC/USD/")))
	if btc == nil {
		t.Fatal("BTC/USD ticker was not indexed by role/symbol")
	}

	scope, _ := btc.Scope()
	if scope != "BTC/USD" {
		t.Fatalf("scope=%q, want BTC/USD", scope)
	}

	var frame struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
		Data    []struct {
			Symbol string  `json:"symbol"`
			Last   float64 `json:"last"`
		} `json:"data"`
	}
	if err := sonic.Unmarshal(btc.DecryptPayload(), &frame); err != nil {
		t.Fatalf("failed to decode scoped payload: %v", err)
	}
	if frame.Channel != "ticker" || frame.Type != "update" {
		t.Fatalf("frame metadata not preserved: channel=%q type=%q", frame.Channel, frame.Type)
	}
	if len(frame.Data) != 1 || frame.Data[0].Symbol != "BTC/USD" || frame.Data[0].Last != 100 {
		t.Fatalf("BTC payload not scoped to one row: %+v", frame.Data)
	}

	if firstArtifact(ws.tree.Seek([]byte("ticker/ETH/EUR/"))) == nil {
		t.Fatal("ETH/EUR ticker was not indexed by role/symbol")
	}
}

func firstArtifact(seq func(func(*datura.Artifact) bool)) *datura.Artifact {
	var first *datura.Artifact
	seq(func(candidate *datura.Artifact) bool {
		first = candidate
		return false
	})
	return first
}
