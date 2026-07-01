package response

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestTreeHandlerIndexesScopedHistoryRoles(t *testing.T) {
	tree := dmt.NewTree("")
	handler := NewTreeHandler(tree)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithPayload([]byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":99,"qty":1}],"asks":[{"price":100,"qty":1}]}]}`))
	artifact.SetTimestamp(time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC).UnixNano())
	defer artifact.Release()

	handler.Send(artifact)

	if got := countTreeArtifacts(tree, []byte("book/BTC/USD/")); got != 1 {
		t.Fatalf("scoped book history count=%d, want 1", got)
	}

	if got := countTreeArtifacts(tree, []byte("book/2026/06/29/12/00/00/BTC/USD/")); got != 1 {
		t.Fatalf("book cursor count=%d, want 1", got)
	}

	if _, ok := tree.Get(latestScopedKey("book", "BTC/USD")); !ok {
		t.Fatal("latest scoped book index missing")
	}
}

func TestTreeHandlerIndexesTickerSnapshotsAsScopedHistory(t *testing.T) {
	tree := dmt.NewTree("")
	handler := NewTreeHandler(tree)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1}]}`))
	artifact.SetTimestamp(time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC).UnixNano())
	defer artifact.Release()

	handler.Send(artifact)

	if got := countTreeArtifacts(tree, []byte("ticker/BTC/USD/")); got != 1 {
		t.Fatalf("ticker scoped history count=%d, want 1", got)
	}

	if got := countTreeArtifacts(tree, []byte("ticker/2026/06/29/12/00/00/BTC/USD/")); got != 1 {
		t.Fatalf("ticker cursor count=%d, want 1", got)
	}

	if _, ok := tree.Get(latestScopedKey("ticker", "BTC/USD")); !ok {
		t.Fatal("latest scoped ticker index missing")
	}
}

func TestTreeHandlerCapturesReplayJSONLWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.jsonl")
	t.Setenv("SYMM_REPLAY_CAPTURE", path)

	tree := dmt.NewTree("")
	handler := NewTreeHandler(tree)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithPayload([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":100,"volume":1}]}`))
	artifact.SetTimestamp(time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC).UnixNano())
	defer artifact.Release()

	handler.Send(artifact)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replay capture: %v", err)
	}

	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatalf("decode replay capture: %v", err)
	}

	if row["role"] != "ticker" || row["scope"] != "BTC/USD" {
		t.Fatalf("unexpected replay row: %#v", row)
	}
	if row["timestamp"] == nil {
		t.Fatalf("replay row missing timestamp: %#v", row)
	}
}

func TestTreeHandlerRejectsInvalidArtifacts(t *testing.T) {
	tree := dmt.NewTree("")
	handler := NewTreeHandler(tree)

	if out := handler.Send(&datura.Artifact{}); out != nil {
		t.Fatalf("invalid artifact returned %#v, want nil", out)
	}
	if got := countTreeArtifacts(tree, []byte{}); got != 0 {
		t.Fatalf("tree artifacts = %d, want 0", got)
	}
}

func countTreeArtifacts(tree *dmt.Tree, prefix []byte) int {
	count := 0

	for range tree.Seek(prefix) {
		count++
	}

	return count
}
